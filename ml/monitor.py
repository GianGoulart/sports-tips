"""
Model drift monitor.
Compares recent Brier score against baseline.
Logs a warning if degradation exceeds 15%.
"""

import sys
from db import get_connection


def get_baseline_brier(conn):
    """Return the best (lowest) Brier score ever recorded."""
    with conn.cursor() as cur:
        cur.execute("SELECT MIN(brier_score) FROM ml_model_metrics")
        row = cur.fetchone()
        return float(row[0]) if row and row[0] is not None else None


def get_recent_brier(conn, days: int = 7):
    """Return the most recent Brier score within the last N days."""
    with conn.cursor() as cur:
        cur.execute("""
            SELECT brier_score FROM ml_model_metrics
            WHERE trained_at > NOW() - INTERVAL '%s days'
            ORDER BY trained_at DESC
            LIMIT 1
        """, (days,))
        row = cur.fetchone()
        return float(row[0]) if row and row[0] is not None else None


def main():
    conn = get_connection()
    baseline = get_baseline_brier(conn)
    recent = get_recent_brier(conn)
    conn.close()

    if baseline is None or recent is None:
        print("Insufficient metric history for drift check.")
        sys.exit(0)

    threshold = baseline * 1.15
    degradation_pct = ((recent - baseline) / baseline) * 100

    print(f"Brier baseline: {baseline:.4f}")
    print(f"Brier recent:   {recent:.4f}")
    print(f"Degradation:    {degradation_pct:+.1f}%")

    if recent > threshold:
        print(f"WARNING: Model drift detected! Brier {recent:.4f} > threshold {threshold:.4f}")
        print("Action: review features and retrain with fresh data.")
        sys.exit(1)
    else:
        print("Model drift: OK")


if __name__ == "__main__":
    main()
