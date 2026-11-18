/** How long the backlog takes to clear at the limit. */
export function clearLabel(days: number): string {
  if (days < 0) return 'growing at this limit';
  return days <= 1 ? 'clear by tomorrow' : `about ${days} days to clear`;
}
