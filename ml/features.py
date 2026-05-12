"""
Feature engineering for match outcome prediction.

Features per match:
  implied_prob_home  - average implied probability from bookmaker odds (home win)
  implied_prob_draw  - average implied probability from bookmaker odds (draw)
  implied_prob_away  - average implied probability from bookmaker odds (away win)
  elo_home           - simple ELO rating for home team (computed from results history)
  elo_away           - simple ELO rating for away team
  elo_diff           - elo_home - elo_away
  home_win_rate_5    - home team win rate in last 5 home matches
  away_win_rate_5    - away team win rate in last 5 away matches
  goal_diff_home_5   - home team avg goal diff in last 5 home matches
  goal_diff_away_5   - away team avg goal diff in last 5 away matches
"""

import pandas as pd
import numpy as np
from db import get_connection

FEATURE_COLS = [
    "implied_prob_home",
    "implied_prob_draw",
    "implied_prob_away",
    "elo_diff",
    "home_win_rate_5",
    "away_win_rate_5",
    "goal_diff_home_5",
    "goal_diff_away_5",
]

TARGET_COL = "outcome_encoded"  # 0=home, 1=draw, 2=away


def fetch_training_data(conn, sports: list = None) -> pd.DataFrame:
    """
    Join matches + results + odds_normalized to build raw training rows.
    Returns one row per match that has a result.
    sports: optional list of sport keys to filter (e.g. ["soccer_epl", "soccer_spain_la_liga"]).
            None = all sports.
    """
    sport_filter = ""
    params = {}
    if sports:
        placeholders = ",".join(f"%(sport_{i})s" for i in range(len(sports)))
        sport_filter = f"AND m.sport IN ({placeholders})"
        params = {f"sport_{i}": s for i, s in enumerate(sports)}

    query = f"""
        SELECT
            m.id          AS match_id,
            m.home_team,
            m.away_team,
            m.starts_at,
            m.sport,
            r.outcome,
            r.score_home,
            r.score_away,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_home, 0) END) AS implied_prob_home,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_draw, 0) END) AS implied_prob_draw,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_away, 0) END) AS implied_prob_away
        FROM matches m
        JOIN results r ON r.match_id = m.id
        JOIN odds_normalized o ON o.match_id = m.id
        WHERE 1=1 {sport_filter}
        GROUP BY m.id, m.home_team, m.away_team, m.starts_at, m.sport, r.outcome, r.score_home, r.score_away
        ORDER BY m.starts_at ASC
    """
    return pd.read_sql(query, conn, params=params if params else None)


def compute_elo(df: pd.DataFrame, k: int = 20) -> pd.DataFrame:
    """
    Compute ELO ratings in chronological order.
    Adds elo_home and elo_away columns. Modifies df in place.
    """
    elo = {}

    def get_elo(team):
        return elo.get(team, 1500.0)

    elo_home_list, elo_away_list = [], []

    for _, row in df.iterrows():
        h, a = row["home_team"], row["away_team"]
        eh, ea = get_elo(h), get_elo(a)
        elo_home_list.append(eh)
        elo_away_list.append(ea)

        exp_h = 1 / (1 + 10 ** ((ea - eh) / 400))
        exp_a = 1 - exp_h

        if row["outcome"] == "home":
            act_h, act_a = 1.0, 0.0
        elif row["outcome"] == "away":
            act_h, act_a = 0.0, 1.0
        else:
            act_h, act_a = 0.5, 0.5

        elo[h] = eh + k * (act_h - exp_h)
        elo[a] = ea + k * (act_a - exp_a)

    df["elo_home"] = elo_home_list
    df["elo_away"] = elo_away_list
    df["elo_diff"] = df["elo_home"] - df["elo_away"]
    return df


def compute_rolling_stats(df: pd.DataFrame, n: int = 5) -> pd.DataFrame:
    """
    Compute rolling win rates and goal diffs per team over last n matches.
    Adds home_win_rate_5, away_win_rate_5, goal_diff_home_5, goal_diff_away_5.
    """
    home_win_rates, away_win_rates = [], []
    home_gdiffs, away_gdiffs = [], []

    team_history = {}

    for idx, row in df.iterrows():
        h, a = row["home_team"], row["away_team"]
        h_hist = team_history.get(h, [])
        a_hist = team_history.get(a, [])

        last_h = h_hist[-n:] if len(h_hist) >= n else h_hist
        last_a = a_hist[-n:] if len(a_hist) >= n else a_hist

        home_win_rates.append(np.mean([x[0] for x in last_h]) if last_h else 0.5)
        away_win_rates.append(np.mean([x[0] for x in last_a]) if last_a else 0.5)
        home_gdiffs.append(np.mean([x[1] for x in last_h]) if last_h else 0.0)
        away_gdiffs.append(np.mean([x[1] for x in last_a]) if last_a else 0.0)

        gd_h = row["score_home"] - row["score_away"]
        gd_a = -gd_h
        is_home_win = 1.0 if row["outcome"] == "home" else 0.0
        is_away_win = 1.0 if row["outcome"] == "away" else 0.0

        team_history.setdefault(h, []).append((is_home_win, gd_h))
        team_history.setdefault(a, []).append((is_away_win, gd_a))

    df["home_win_rate_5"] = home_win_rates
    df["away_win_rate_5"] = away_win_rates
    df["goal_diff_home_5"] = home_gdiffs
    df["goal_diff_away_5"] = away_gdiffs
    return df


def encode_outcome(df: pd.DataFrame) -> pd.DataFrame:
    """Encode outcome string -> int: home=0, draw=1, away=2."""
    mapping = {"home": 0, "draw": 1, "away": 2}
    df[TARGET_COL] = df["outcome"].map(mapping)
    return df


def build_dataset(conn, sports: list = None) -> tuple:
    """
    Full pipeline: fetch -> ELO -> rolling stats -> encode -> return X, y.
    sports: optional sport key filter passed to fetch_training_data.
    """
    df = fetch_training_data(conn, sports=sports)
    if df.empty:
        return pd.DataFrame(columns=FEATURE_COLS), pd.Series(dtype=int)

    df = compute_elo(df)
    df = compute_rolling_stats(df)
    df = encode_outcome(df)

    df = df.dropna(subset=FEATURE_COLS + [TARGET_COL])

    X = df[FEATURE_COLS]
    y = df[TARGET_COL].astype(int)
    return X, y
