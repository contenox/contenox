import { describe, expect, it } from "vitest";
import { formatCompactNumber } from "./format";

describe("formatCompactNumber", () => {
  it("leaves small numbers unabbreviated", () => {
    expect(formatCompactNumber(0)).toBe("0");
    expect(formatCompactNumber(950, "en-US")).toBe("950");
  });

  it("abbreviates thousands with one fraction digit", () => {
    expect(formatCompactNumber(1500, "en-US")).toBe("1.5K");
  });

  it("abbreviates millions with one fraction digit", () => {
    expect(formatCompactNumber(1_000_000, "en-US")).toBe("1M");
    expect(formatCompactNumber(1_500_000, "en-US")).toBe("1.5M");
    expect(formatCompactNumber(12_345_678, "en-US")).toBe("12.3M");
  });

  it("preserves the sign for negative values", () => {
    expect(formatCompactNumber(-2500, "en-US")).toBe("-2.5K");
  });

  it("is locale-aware", () => {
    // German compact notation joins the number and unit with a non-breaking
    // space, so match loosely rather than pin the exact whitespace byte.
    expect(formatCompactNumber(1_500_000, "de-DE")).toMatch(/^1,5\s*Mio\.$/);
  });

  it("does not throw on non-finite input", () => {
    expect(formatCompactNumber(Number.NaN, "en-US")).toBe("NaN");
    expect(formatCompactNumber(Number.POSITIVE_INFINITY, "en-US")).toBe("∞");
  });
});
