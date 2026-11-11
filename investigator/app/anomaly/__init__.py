"""Phase 11 anomaly detection package (FRACTAL).

Trains and exports anomaly models; runtime inference happens in Go from the
exported artifacts under api/internal/anomaly/artifacts/.

IMPORTANT: all data produced by app.anomaly.synthetic is SYNTHETIC and is
labeled as such. Metrics reported by app.anomaly.evaluate are held-out
SYNTHETIC metrics and must never be presented as real-world accuracy.
"""
