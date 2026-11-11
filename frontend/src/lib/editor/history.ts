/**
 * Historique d'annulation / rétablissement de l'éditeur.
 *
 * L'annulation native du champ de saisie ne couvre pas les modifications
 * faites par la barre d'outils (écriture programmatique) et ne survit pas
 * au rendu piloté par le store : l'éditeur garde donc ses propres
 * instantanés du contenu.
 */

export interface HistorySnapshot {
  value: string;
}

/** Ce dont l'historique a besoin de l'éditeur, injecté pour rester testable. */
export interface HistoryIO {
  /** Identifiant de la note courante (vide = rien d'ouvert). */
  noteId(): string;
  /** Contenu courant de la note. */
  content(): string;
  /** Applique un contenu (typiquement updateSelected({ content })). */
  apply(next: string): void;
  /** Replace le curseur après une restauration (index dans le contenu). */
  focus(caret: number): void;
}

const HISTORY_LIMIT = 200;
/** Frappes rapprochées : une seule étape d'annulation. */
const GROUP_MS = 700;

export class EditorHistory {
  private undoStack: HistorySnapshot[] = [];
  private redoStack: HistorySnapshot[] = [];
  private historyNoteId = '';
  private lastEditAt = 0;

  constructor(private readonly io: HistoryIO) {}

  private ensure(): void {
    const id = this.io.noteId();
    if (!id || id === this.historyNoteId) return;
    this.historyNoteId = id;
    this.undoStack = [];
    this.redoStack = [];
    this.lastEditAt = 0;
  }

  /** À appeler AVANT de remplacer le contenu. `group` fusionne les frappes. */
  recordBeforeChange(group: boolean): void {
    if (!this.io.noteId()) return;
    this.ensure();
    const now = Date.now();
    if (!group || this.undoStack.length === 0 || now - this.lastEditAt > GROUP_MS) {
      this.undoStack.push({ value: this.io.content() });
      if (this.undoStack.length > HISTORY_LIMIT) this.undoStack.shift();
    }
    this.redoStack = [];
    this.lastEditAt = now;
  }

  /** Remplace le contenu en conservant une étape d'annulation. */
  applyContent(next: string, group = false): void {
    if (!this.io.noteId() || next === this.io.content()) return;
    this.recordBeforeChange(group);
    this.io.apply(next);
  }

  private restore(from: HistorySnapshot[], to: HistorySnapshot[]): void {
    if (!this.io.noteId()) return;
    this.ensure();
    const target = from.pop();
    if (!target) return;
    const current = this.io.content();
    to.push({ value: current });
    this.io.apply(target.value);
    this.lastEditAt = 0; // la frappe suivante ouvre une nouvelle étape

    // Curseur placé à la fin de la zone modifiée.
    let suffix = 0;
    const max = Math.min(current.length, target.value.length);
    while (
      suffix < max &&
      current[current.length - 1 - suffix] === target.value[target.value.length - 1 - suffix]
    ) {
      suffix++;
    }
    this.io.focus(target.value.length - suffix);
  }

  undo(): void {
    this.restore(this.undoStack, this.redoStack);
  }

  redo(): void {
    this.restore(this.redoStack, this.undoStack);
  }
}
