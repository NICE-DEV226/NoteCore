/**
 * Révision espacée (ADR-007) : statistiques, maîtrise et séance en cours.
 * Les cartes sont dérivées des notes par le backend ; ce store ne fait que
 * présenter la file d'attente et transmettre les réponses.
 */

import { get, writable } from 'svelte/store';
import { api } from '../api/client';
import type { MasteryRow, ReviewCard, ReviewRating, ReviewStats } from '../api/types';

export const reviewStats = writable<ReviewStats | null>(null);
export const mastery = writable<MasteryRow[]>([]);

/** Séance en plein écran ouverte ? */
export const reviewOpen = writable(false);
export const currentCard = writable<ReviewCard | null>(null);
export const revealed = writable(false);
export const reviewBusy = writable(false);
export const reviewError = writable<string | null>(null);

export interface SessionTally {
  answered: number;
  again: number;
  hard: number;
  good: number;
  easy: number;
}
const EMPTY: SessionTally = { answered: 0, again: 0, hard: 0, good: 0, easy: 0 };
export const tally = writable<SessionTally>({ ...EMPTY });
/** Réponses données dans la séance, pour pouvoir annuler la dernière. */
export const undoable = writable(false);

let refreshTimer: ReturnType<typeof setTimeout> | null = null;

export async function refreshReview(): Promise<void> {
  try {
    const [stats, rows] = await Promise.all([api.reviewStats(), api.reviewMastery()]);
    reviewStats.set(stats);
    mastery.set(rows);
  } catch (err) {
    console.warn('Statistiques de révision indisponibles :', err);
  }
}

/** Rafraîchit sans bruit après une modification de note (les cartes suivent le texte). */
export function scheduleReviewRefresh(delayMs = 1500): void {
  if (refreshTimer !== null) clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => {
    refreshTimer = null;
    void refreshReview();
  }, delayMs);
}

async function loadNext(): Promise<void> {
  reviewError.set(null);
  try {
    currentCard.set(await api.reviewNext());
    revealed.set(false);
  } catch (err) {
    reviewError.set(err instanceof Error ? err.message : 'Impossible de charger la carte suivante.');
  }
}

export async function startReview(): Promise<void> {
  tally.set({ ...EMPTY });
  undoable.set(false);
  reviewOpen.set(true);
  reviewBusy.set(true);
  await loadNext();
  reviewBusy.set(false);
}

export function reveal(): void {
  if (get(currentCard)) revealed.set(true);
}

export async function answerCard(rating: ReviewRating): Promise<void> {
  const card = get(currentCard);
  if (!card || !get(revealed) || get(reviewBusy)) return;
  reviewBusy.set(true);
  try {
    await api.reviewAnswer(card.id, rating);
    tally.update((t) => ({
      answered: t.answered + 1,
      again: t.again + (rating === 1 ? 1 : 0),
      hard: t.hard + (rating === 2 ? 1 : 0),
      good: t.good + (rating === 3 ? 1 : 0),
      easy: t.easy + (rating === 4 ? 1 : 0)
    }));
    undoable.set(true);
    await loadNext();
    void refreshReview();
  } catch (err) {
    reviewError.set(err instanceof Error ? err.message : "La réponse n'a pas pu être enregistrée.");
  } finally {
    reviewBusy.set(false);
  }
}

export async function undoAnswer(): Promise<void> {
  if (!get(undoable) || get(reviewBusy)) return;
  reviewBusy.set(true);
  try {
    const card = await api.reviewUndo();
    if (card) {
      currentCard.set(card);
      revealed.set(false);
      tally.update((t) => ({ ...t, answered: Math.max(0, t.answered - 1) }));
    }
    undoable.set(false);
    void refreshReview();
  } catch (err) {
    reviewError.set(err instanceof Error ? err.message : "L'annulation a échoué.");
  } finally {
    reviewBusy.set(false);
  }
}

export function closeReview(): void {
  reviewOpen.set(false);
  currentCard.set(null);
  revealed.set(false);
  void refreshReview();
}
