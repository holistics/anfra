// Typo suggestions for validation messages: an author's (or agent's) misspelt name, and the
// closest one the dataset has.

/**
 * Levenshtein distance, capped: only used to suggest a near-miss field name, so exact cost past a
 * couple of edits is irrelevant.
 */
function editDistance (a: string, b: string): number {
  const rows = a.length + 1;
  const cols = b.length + 1;
  let prev = Array.from({ length: cols }, (_, j) => j);

  for (let i = 1; i < rows; i++) {
    const curr = [i];
    for (let j = 1; j < cols; j++) {
      curr[j] = Math.min(
        prev[j] + 1,
        curr[j - 1] + 1,
        prev[j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1),
      );
    }
    prev = curr;
  }

  return prev[cols - 1];
}

/** Returns the closest candidate within a plausible typo distance, or undefined. */
export function suggest (value: string, candidates: string[]): string | undefined {
  let best: string | undefined;
  let bestDistance = Infinity;

  candidates.forEach((candidate) => {
    const distance = editDistance(value.toLowerCase(), candidate.toLowerCase());
    if (distance < bestDistance) {
      bestDistance = distance;
      best = candidate;
    }
  });

  const threshold = Math.max(2, Math.floor(value.length / 3));
  return bestDistance <= threshold ? best : undefined;
}

export function withSuggestion (message: string, value: string, candidates: string[]): string {
  const hint = suggest(value, candidates);
  return hint ? `${message} Did you mean '${hint}'?` : message;
}
