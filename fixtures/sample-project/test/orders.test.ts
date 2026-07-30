import { describe, it, expect, vi, beforeEach } from "vitest";
import { createOrder, total, count, reset } from "../src/orders.ts";

beforeEach(() => reset());

describe("orders", () => {
  // covers: SC-01
  it("rejects zero quantity", () => {
    expect(() => createOrder(0)).toThrow("qty must be > 0");
    expect(count()).toBe(0);
  });

  // covers: SC-02
  it.skip("rolls back on mid-write failure", () => {
    expect(() =>
      createOrder(3, () => {
        throw new Error("boom");
      }),
    ).toThrow("boom");
    expect(count()).toBe(0);
  });

  // covers: SC-03
  it("creates an order", () => {
    const spy = vi.fn();
    createOrder(2, spy);
    expect(spy).toHaveBeenCalledOnce();
  });

  it("computes total", () => {
    createOrder(2);
    createOrder(3);
    expect(total()).toBe(5);
  });
});
