PRAGMA foreign_keys = ON;

CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK(status IN ('active', 'disabled'))
);

CREATE TABLE orders (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  total_cents INTEGER NOT NULL CHECK(total_cents >= 0),
  state TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE order_items (
  id INTEGER PRIMARY KEY,
  order_id INTEGER NOT NULL,
  sku TEXT NOT NULL,
  quantity INTEGER NOT NULL CHECK(quantity > 0),
  unit_cents INTEGER NOT NULL CHECK(unit_cents >= 0),
  FOREIGN KEY(order_id) REFERENCES orders(id)
);

CREATE UNIQUE INDEX idx_orders_state_created_at ON orders(state, created_at);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_order_items_order_id ON order_items(order_id);

CREATE VIEW v_open_orders AS
SELECT o.id, o.user_id, o.total_cents, o.state
FROM orders o
WHERE o.state IN ('pending', 'processing');

CREATE TRIGGER trg_orders_touch
AFTER UPDATE ON orders
FOR EACH ROW
BEGIN
  UPDATE orders SET created_at = NEW.created_at WHERE id = NEW.id;
END;

INSERT INTO users(id, username, email, status) VALUES
  (1, 'alice', 'alice@example.com', 'active'),
  (2, 'bob', 'bob@example.com', 'active');

INSERT INTO orders(id, user_id, total_cents, state, created_at) VALUES
  (100, 1, 2599, 'pending', '2026-02-26T10:00:00Z'),
  (101, 2, 1099, 'processing', '2026-02-26T10:05:00Z');

INSERT INTO order_items(id, order_id, sku, quantity, unit_cents) VALUES
  (1000, 100, 'SKU-RED-01', 1, 2599),
  (1001, 101, 'SKU-BLU-02', 1, 1099);
