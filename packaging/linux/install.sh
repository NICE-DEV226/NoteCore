#!/bin/sh
# Installation de NoteCore sans droits administrateur (ou pour tout le système).
#
#   ./install.sh              installe pour l'utilisateur courant (~/.local)
#   ./install.sh --system     installe pour tous (/usr/local, demande sudo)
#   ./install.sh --uninstall  désinstalle (ajouter --system si installé ainsi)
#
# Les notes (~/.config/NoteCore) ne sont jamais touchées, ni à l'installation,
# ni à la mise à jour, ni à la désinstallation.
set -eu

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
MODE=install
SCOPE=user

for arg in "$@"; do
  case "$arg" in
    --system) SCOPE=system ;;
    --uninstall) MODE=uninstall ;;
    -h|--help) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "Option inconnue : $arg (voir --help)" >&2; exit 2 ;;
  esac
done

if [ "$SCOPE" = system ]; then
  PREFIX=/usr/local
  SUDO=
  [ "$(id -u)" -eq 0 ] || SUDO=sudo
else
  PREFIX="$HOME/.local"
  SUDO=
fi

BIN="$PREFIX/bin"
APPS="$PREFIX/share/applications"
ICONS="$PREFIX/share/icons/hicolor/256x256/apps"
META="$PREFIX/share/metainfo"

if [ "$MODE" = uninstall ]; then
  $SUDO rm -f "$BIN/notecore" "$APPS/notecore.desktop" "$ICONS/notecore.png" \
    "$META/notecore.metainfo.xml"
  echo "NoteCore est désinstallé. Vos notes sont conservées dans ~/.config/NoteCore."
  exit 0
fi

# NoteCore s'affiche avec WebKitGTK 4.1 et GTK 3 : on le vérifie avant de
# copier quoi que ce soit, pour donner une consigne précise plutôt qu'une
# erreur de bibliothèque au premier lancement.
have_lib() { ldconfig -p 2>/dev/null | grep -q "$1"; }
if ! have_lib libwebkit2gtk-4.1 || ! have_lib libgtk-3; then
  echo "Bibliothèques manquantes : WebKitGTK 4.1 et GTK 3 sont requis." >&2
  if [ -r /etc/os-release ]; then . /etc/os-release; fi
  case "${ID:-} ${ID_LIKE:-}" in
    *arch*)            echo "  sudo pacman -S webkit2gtk-4.1 gtk3" >&2 ;;
    *debian*|*ubuntu*) echo "  sudo apt install libwebkit2gtk-4.1-0 libgtk-3-0" >&2 ;;
    *fedora*|*rhel*)   echo "  sudo dnf install webkit2gtk4.1 gtk3" >&2 ;;
    *suse*)            echo "  sudo zypper install libwebkit2gtk-4_1-0 gtk3" >&2 ;;
    *)                 echo "  Installez les paquets WebKitGTK 4.1 et GTK 3 de votre distribution." >&2 ;;
  esac
  echo "Puis relancez ./install.sh." >&2
  exit 1
fi

for f in notecore notecore.png notecore.desktop; do
  [ -f "$HERE/$f" ] || { echo "Fichier manquant dans l'archive : $f" >&2; exit 1; }
done

$SUDO install -Dm755 "$HERE/notecore" "$BIN/notecore"
$SUDO install -Dm644 "$HERE/notecore.png" "$ICONS/notecore.png"
[ -f "$HERE/notecore.metainfo.xml" ] && $SUDO install -Dm644 "$HERE/notecore.metainfo.xml" "$META/notecore.metainfo.xml"

# Le lanceur pointe vers le binaire installé (chemin absolu : fonctionne même
# si ~/.local/bin n'est pas dans le PATH).
TMP=$(mktemp)
sed "s|^Exec=.*|Exec=$BIN/notecore|" "$HERE/notecore.desktop" > "$TMP"
$SUDO install -Dm644 "$TMP" "$APPS/notecore.desktop"
rm -f "$TMP"

command -v update-desktop-database >/dev/null 2>&1 && $SUDO update-desktop-database "$APPS" >/dev/null 2>&1 || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && $SUDO gtk-update-icon-cache -q -t "$PREFIX/share/icons/hicolor" >/dev/null 2>&1 || true

echo "NoteCore est installé : $BIN/notecore"
case ":$PATH:" in
  *":$BIN:"*) ;;
  *) echo "Astuce : $BIN n'est pas dans votre PATH ; le lanceur du menu fonctionne quand même." ;;
esac
echo "Vos notes sont dans ~/.config/NoteCore (jamais modifiées par ce script)."
