"""Shared pytest fixtures for the Phase 11 anomaly tests."""
import os
import sys

# Make `app` importable when pytest is run from the repo root or investigator/.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
