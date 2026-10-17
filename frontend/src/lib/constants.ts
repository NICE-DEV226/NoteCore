/**
 * Constantes partagées de l'interface.
 */

/**
 * Id du champ de recherche de la zone B. Partagé entre la liste
 * (`NoteList.svelte`, qui rend l'input) et la fenêtre principale
 * (`Workbench.svelte`, qui le cible via Ctrl/Cmd+K — cahier des charges §3.6).
 */
export const SEARCH_INPUT_ID = 'notecore-search';
