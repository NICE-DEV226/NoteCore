/** Aides d'écriture des cartes de révision (syntaxe : voir ADR-007). */

/** Plus petit numéro de trou libre : « {{c1::…}} », « {{c2::…}} »… */
export function nextClozeNumber(content: string): number {
  let max = 0;
  for (const m of content.matchAll(/\{\{c(\d{1,2})::/g)) max = Math.max(max, Number(m[1]));
  return max + 1;
}
