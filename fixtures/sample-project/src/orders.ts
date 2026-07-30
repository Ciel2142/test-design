export type Order = { id: number; qty: number };

const rows: Order[] = [];
let nextId = 1;

export function createOrder(qty: number, onWrite?: () => void): Order {
  if (qty <= 0) {
    throw new Error("qty must be > 0");
  }
  const order = { id: nextId++, qty };
  rows.push(order);
  try {
    onWrite?.();
  } catch (err) {
    rows.pop();
    throw err;
  }
  return order;
}

export function total(): number {
  return rows.reduce((sum, r) => sum + r.qty, 0);
}

export function count(): number {
  return rows.length;
}

export function reset(): void {
  rows.length = 0;
  nextId = 1;
}
