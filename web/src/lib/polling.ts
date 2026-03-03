export function nextPollMs(minMs: number, maxMs: number): number {
  const min = Math.max(1, Math.floor(minMs))
  const max = Math.max(min, Math.floor(maxMs))
  if (min === max) {
    return min
  }
  return Math.floor(Math.random() * (max - min + 1)) + min
}
