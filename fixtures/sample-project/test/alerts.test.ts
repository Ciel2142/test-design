import { describe, it, expect } from "vitest";
import { createOrder, total, reset } from "../src/orders.ts";

// No `covers:` tags in this file. It exists as a decoy for layer 1's join. Do not rename
// it: `alerts` sorts before `orders`, so this passing row reaches the runner report first.
//
// The test below carries the same name as SC-02's test in orders.test.ts, and it passes
// while SC-02's is `it.skip`. A join on test name alone matches both rows with no rule for
// choosing, takes this one, and reports SC-02 covered — the silent false pass layer 1
// exists to prevent. Joining on the tag's file as well as its name pins SC-02 to
// orders.test.ts and yields `gap`.

describe("alerts", () => {
  it("rolls back on mid-write failure", () => {
    reset();
    createOrder(2);
    expect(total()).toBe(2);
  });
});
