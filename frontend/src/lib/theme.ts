export const classColors: Record<number, string> = {
  1: "#C69B6D",
  2: "#F48CBA",
  3: "#AAD372",
  4: "#FFF468",
  5: "#FFFFFF",
  6: "#C41E3A",
  7: "#0070DD",
  8: "#3FC7EB",
  9: "#9482C9",
  10: "#00FF98",
  11: "#FF7C0A",
  12: "#A330C9",
  13: "#33937F",
};
export const classColor = (id?: number) => classColors[id ?? 0] ?? "#94a3a0";
