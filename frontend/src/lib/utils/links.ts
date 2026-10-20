import { api } from '../api/client';
import { openAttachmentViewer } from '../stores/files';

/**
 * Aucun lien ne doit faire naviguer la fenêtre de l'application : elle n'a
 * ni barre d'adresse ni bouton « retour », l'utilisateur serait coincé hors
 * de l'interface. Un lien externe s'ouvre dans le navigateur du système, un
 * lien de pièce jointe s'ouvre dans la visionneuse, tout le reste est ignoré.
 * L'écoute est globale et en capture : elle couvre aussi les clics du
 * milieu et ne dépend d'aucun composant.
 */
export function installLinkGuard(): void {
  const handle = (event: MouseEvent): void => {
    const target = event.target as Element | null;

    // Image jointe dans l'aperçu : un clic la lit en grand (zoom, navigation).
    const image = target?.closest?.('img[data-attachment]') as HTMLImageElement | null;
    if (image && event.type === 'click') {
      event.preventDefault();
      if (image.dataset.attachment) void openAttachmentViewer(image.dataset.attachment);
      return;
    }

    const link = target?.closest?.('a') as HTMLAnchorElement | null;
    if (!link) return;
    event.preventDefault();
    if (event.type === 'auxclick') return;

    const attachment = link.dataset.attachment;
    if (attachment) {
      // Un lien de pièce jointe se lit dans NoteCore ; « Enregistrer » et
      // « Ouvrir » sont proposés par la visionneuse.
      void openAttachmentViewer(attachment);
      return;
    }
    const href = link.getAttribute('href') ?? '';
    if (/^(https?:|mailto:)/i.test(href)) void api.openExternal(href);
  };
  document.addEventListener('click', handle, true);
  document.addEventListener('auxclick', handle, true);
}
