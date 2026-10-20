package share

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Envoi direct sur le réseau local, SANS Internet : un point d'accès, un
// partage de connexion ou un simple Wi-Fi de salle suffisent. NoteCore ouvre un
// serveur éphémère qui ne sert qu'UN fichier, derrière un jeton aléatoire, à
// UN téléchargement, pendant quelques minutes.

// ErrNoNetwork : aucune interface réseau locale utilisable.
var ErrNoNetwork = errors.New("aucun réseau local détecté : connectez les deux appareils au même Wi-Fi (Internet n'est pas nécessaire) ou activez un point d'accès")

const (
	// DefaultTTL : durée pendant laquelle l'envoi reste disponible.
	DefaultTTL = 10 * time.Minute
	// stopDelay laisse au client le temps de finir de lire la réponse.
	stopDelay = 1500 * time.Millisecond
)

// SessionInfo décrit un envoi en cours.
type SessionInfo struct {
	URLs      []string `json:"urls"` // adresses à ouvrir ou à scanner
	Filename  string   `json:"filename"`
	Size      int      `json:"size"`
	ExpiresAt string   `json:"expiresAt"`
	Encrypted bool     `json:"encrypted"`
}

// Status est l'état de l'envoi, relu par l'interface.
type Status struct {
	State string `json:"state"` // idle | waiting | downloaded | expired | stopped
	SessionInfo
}

type session struct {
	token     string
	data      atomic.Pointer[[]byte] // vidé à la clôture : les notes ne restent pas en mémoire
	filename  string
	encrypted bool
	srv       *http.Server
	timer     *time.Timer
	info      SessionInfo
	state     string
}

// Manager gère l'unique envoi actif.
type Manager struct {
	mu            sync.Mutex
	cur           *session
	allowLoopback bool // tests
}

func NewManager() *Manager { return &Manager{} }

func localHosts(allowLoopback bool) []string {
	var hosts []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		if ifc.Flags&net.FlagLoopback != 0 && !allowLoopback {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip := ipn.IP
			if ip.IsLoopback() && !allowLoopback {
				continue
			}
			if ip.IsPrivate() || ip.IsLinkLocalUnicast() || (allowLoopback && ip.IsLoopback()) {
				hosts = append(hosts, ip.String())
			}
		}
	}
	return hosts
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Start ouvre l'envoi. Un envoi précédent est fermé d'abord.
func (m *Manager) Start(data []byte, filename string, encrypted bool, ttl time.Duration) (SessionInfo, error) {
	hosts := localHosts(m.allowLoopback)
	if len(hosts) == 0 {
		return SessionInfo{}, ErrNoNetwork
	}
	token, err := newToken()
	if err != nil {
		return SessionInfo{}, fmt.Errorf("jeton : %w", err)
	}
	ln, err := net.Listen("tcp4", ":0")
	if err != nil {
		return SessionInfo{}, fmt.Errorf("ouverture du port : %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	if ttl <= 0 {
		ttl = DefaultTTL
	}
	expires := time.Now().Add(ttl)
	s := &session{token: token, filename: filename, encrypted: encrypted, state: "waiting"}
	s.data.Store(&data)
	for _, h := range hosts {
		s.info.URLs = append(s.info.URLs, "http://"+net.JoinHostPort(h, strconv.Itoa(port))+"/s/"+token)
	}
	s.info.Filename, s.info.Size, s.info.Encrypted = filename, len(data), encrypted
	s.info.ExpiresAt = expires.UTC().Format(time.RFC3339)

	mux := http.NewServeMux()
	mux.HandleFunc("/s/", func(w http.ResponseWriter, r *http.Request) { m.serve(s, w, r) })
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Minute}

	m.mu.Lock()
	m.stopLocked("stopped")
	m.cur = s
	m.mu.Unlock()

	go func() { _ = s.srv.Serve(ln) }()
	s.timer = time.AfterFunc(ttl, func() { m.finish(s, "expired") })
	return s.info, nil
}

func (m *Manager) serve(s *session, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	tok, sub, _ := strings.Cut(rest, "/")
	if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
		http.NotFound(w, r) // aucun indice sur l'existence d'un envoi
		return
	}

	m.mu.Lock()
	live := m.cur == s && s.state == "waiting"
	m.mu.Unlock()
	payload := s.data.Load()
	if !live || payload == nil {
		http.Error(w, "Cet envoi n'est plus disponible.", http.StatusGone)
		return
	}

	if sub == "" { // page d'accueil : un bouton, lisible sur un téléphone
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html lang="fr"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>NoteCore</title><body style="font-family:system-ui;max-width:28rem;margin:3rem auto;padding:0 1rem;line-height:1.5">
<h1>NoteCore</h1><p>Un appareil vous envoie des notes (%s).</p>
<p><a href="download" style="display:inline-block;padding:.7rem 1.2rem;background:#4f46e5;color:#fff;border-radius:8px;text-decoration:none">Télécharger</a></p>
<p style="color:#666;font-size:.9rem">Le lien ne fonctionne qu'une fois et expire dans quelques minutes.</p></body></html>`,
			html.EscapeString(s.filename))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	body := *payload
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(s.filename))
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		return // téléchargement interrompu : l'envoi reste disponible
	}
	m.finish(s, "downloaded")
}

// finish clôt l'envoi (une seule fois) avec l'état donné.
func (m *Manager) finish(s *session, state string) {
	m.mu.Lock()
	if m.cur != s || s.state != "waiting" {
		m.mu.Unlock()
		return
	}
	s.state = state
	if s.timer != nil {
		s.timer.Stop()
	}
	m.mu.Unlock()
	go func() {
		time.Sleep(stopDelay)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(ctx)
		s.data.Store(nil)
	}()
}

func (m *Manager) stopLocked(state string) {
	s := m.cur
	if s == nil || s.state != "waiting" {
		return
	}
	s.state = state
	if s.timer != nil {
		s.timer.Stop()
	}
	srv := s.srv
	go func() { _ = srv.Close(); s.data.Store(nil) }()
}

// Stop ferme l'envoi en cours.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked("stopped")
}

// Status renvoie l'état courant ; le dernier envoi reste lisible jusqu'au suivant.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		return Status{State: "idle"}
	}
	return Status{State: m.cur.state, SessionInfo: m.cur.info}
}

// Réception -----------------------------------------------------------------

// ErrNotLocal : l'adresse n'est pas sur le réseau local.
var ErrNotLocal = errors.New("adresse refusée : elle n'est pas sur le réseau local (NoteCore ne se connecte jamais à Internet)")

// loopbackAllowed n'est vrai que dans les tests : l'envoi à soi-même n'a pas de
// sens, et le bouclage est la porte d'entrée classique des attaques par
// redirection vers un service local.
var loopbackAllowed = false

func isLocalIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return loopbackAllowed
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// resolveLocal résout un nom et refuse tout ce qui n'est pas sur le réseau
// local — si UNE seule adresse renvoyée est publique, tout est refusé.
func resolveLocal(ctx context.Context, host string) ([]net.IP, error) {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("adresse introuvable : %w", err)
		}
		for _, r := range resolved {
			ips = append(ips, r.IP)
		}
	}
	if len(ips) == 0 {
		return nil, ErrNotLocal
	}
	for _, ip := range ips {
		if !isLocalIP(ip) {
			return nil, ErrNotLocal
		}
	}
	return ips, nil
}

// dialLocal établit la connexion en se servant UNIQUEMENT d'une adresse qu'il
// vient de valider : la vérification et la connexion utilisent la même
// résolution, un serveur DNS malveillant ne peut donc pas répondre « local »
// à la vérification puis « public » à la connexion (DNS rebinding).
func dialLocal(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := resolveLocal(ctx, host)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	var last error
	for _, ip := range ips {
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

// Fetch télécharge un envoi d'un autre appareil. Seules les adresses du
// réseau local sont acceptées, sans proxy ni redirection : l'application
// garde sa promesse de ne jamais contacter Internet.
func Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return nil, errors.New("adresse invalide : collez le lien affiché par l'autre appareil")
	}
	if u.Scheme != "http" {
		return nil, errors.New("adresse invalide : le lien doit commencer par http://")
	}
	if !strings.HasPrefix(u.Path, "/s/") {
		return nil, errors.New("adresse invalide : ce n'est pas un lien d'envoi NoteCore")
	}
	if parts := strings.Split(strings.Trim(u.Path, "/"), "/"); len(parts) == 2 { // /s/<jeton> → fichier
		u.Path = "/s/" + parts[1] + "/download"
	}

	// Contrôle préalable, pour un message clair ; la connexion elle-même est
	// verrouillée par dialLocal (voir plus haut).
	if _, err := resolveLocal(ctx, u.Hostname()); err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout:       2 * time.Minute,
		Transport:     &http.Transport{Proxy: nil, DialContext: dialLocal},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connexion impossible : %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusGone:
		return nil, errors.New("cet envoi a déjà été utilisé ou a expiré : demandez-en un nouveau")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("l'autre appareil a répondu %d : vérifiez le lien", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxPackageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("téléchargement interrompu : %w", err)
	}
	if len(data) > MaxPackageBytes {
		return nil, fmt.Errorf("envoi trop volumineux (maximum %d Mio)", MaxPackageBytes>>20)
	}
	return data, nil
}
