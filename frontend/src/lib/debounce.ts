export type Debounced<A extends unknown[]> = ((...args: A) => void) & { cancel: () => void };

// Returns a wrapper that only calls fn once input has been quiet for waitMs.
// The latest arguments win; cancel() drops any pending invocation.
export function debounce<A extends unknown[]>(
  fn: (...args: A) => void,
  waitMs: number,
): Debounced<A> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const wrapped = ((...args: A) => {
    if (timer !== undefined) clearTimeout(timer);
    timer = setTimeout(() => fn(...args), waitMs);
  }) as Debounced<A>;
  wrapped.cancel = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
  };
  return wrapped;
}
