package share

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func startLoopback(t *testing.T, data []byte, ttl time.Duration) (*Manager, SessionInfo) {
	t.Helper()
	m := &Manager{allowLoopback: true}
	info, err := m.Start(data, "essai.notecore", false, ttl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	return m, info
}

func waitState(m *Manager, want string) bool {
	for i := 0; i < 100; i++ {
		if m.Status().State == want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestEnvoiUnSeulTelechargement(t *testing.T) {
	payload := []byte("paquet de notes")
	m, info := startLoopback(t, payload, time.Minute)
	if len(info.URLs) == 0 || !strings.Contains(info.URLs[0], "/s/") {
		t.Fatalf("adresses = %v", info.URLs)
	}

	got, err := Fetch(context.Background(), info.URLs[0]) // le lien de la page suffit
	if err != nil || string(got) != string(payload) {
		t.Fatalf("réception : %q, %v", got, err)
	}
	if !waitState(m, "downloaded") {
		t.Fatalf("état = %q, attendu downloaded", m.Status().State)
	}
	if _, err := Fetch(context.Background(), info.URLs[0]); err == nil {
		t.Error("le lien ne doit servir qu'une fois")
	}
}

func TestEnvoiRefuseLesMauvaisAcces(t *testing.T) {
	_, info := startLoopback(t, []byte("x"), time.Minute)
	base := info.URLs[0]
	wrong := base[:strings.LastIndex(base, "/")+1] + "mauvais-jeton"

	for _, c := range []struct {
		method, url string
		want        int
	}{
		{"GET", wrong, 404},
		{"GET", wrong + "/download", 404},
		{"POST", base + "/download", 404},
		{"GET", base, 200}, // page d'accueil
	} {
		req, _ := http.NewRequest(c.method, c.url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s %s : %d, attendu %d", c.method, c.url, resp.StatusCode, c.want)
		}
		if c.want == 404 && len(body) > 20 {
			t.Errorf("la réponse d'erreur ne doit rien révéler : %q", body)
		}
	}
}

func TestEnvoiExpireEtSArrete(t *testing.T) {
	m, info := startLoopback(t, []byte("x"), 150*time.Millisecond)
	if !waitState(m, "expired") {
		t.Fatalf("état = %q, attendu expired", m.Status().State)
	}
	time.Sleep(stopDelay + 300*time.Millisecond)
	if _, err := Fetch(context.Background(), info.URLs[0]); err == nil {
		t.Error("un envoi expiré ne doit plus répondre")
	}

	m2, _ := startLoopback(t, []byte("y"), time.Minute)
	m2.Stop()
	if m2.Status().State != "stopped" {
		t.Errorf("état = %q, attendu stopped", m2.Status().State)
	}
	if (&Manager{}).Status().State != "idle" {
		t.Error("sans envoi, l'état est idle")
	}
}

func TestUnNouvelEnvoiRemplaceLePrecedent(t *testing.T) {
	m := &Manager{allowLoopback: true}
	first, _ := m.Start([]byte("un"), "a.notecore", false, time.Minute)
	second, err := m.Start([]byte("deux"), "b.notecore", false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	time.Sleep(100 * time.Millisecond)
	if _, err := Fetch(context.Background(), first.URLs[0]); err == nil {
		t.Error("le premier envoi doit être fermé")
	}
	if got, err := Fetch(context.Background(), second.URLs[0]); err != nil || string(got) != "deux" {
		t.Errorf("second envoi : %q %v", got, err)
	}
}

func TestReceptionRefuseInternet(t *testing.T) {
	for _, u := range []string{
		"http://8.8.8.8/s/abc",
		"http://1.1.1.1:80/s/abc/download",
		"https://192.168.1.2/s/abc", // https : refusé, le réseau local n'a pas de certificat
		"http://192.168.1.2/autre/chemin",
		"ftp://192.168.1.2/s/abc",
		"pas une adresse",
		"",
	} {
		if _, err := Fetch(context.Background(), u); err == nil {
			t.Errorf("%q doit être refusé", u)
		}
	}
	if _, err := Fetch(context.Background(), "http://8.8.8.8/s/abc"); err != ErrNotLocal {
		t.Errorf("hors réseau local : %v", err)
	}
}
