"""Model C — EXPERIMENTAL: VAE reconstruction error + supervised classifier.

Per plan Phase 11: "VAE (reconstruction error and latent features) feeding a
supervised classifier. Benchmark against the baselines; keep it only if it
earns its complexity."

Design (kept deliberately small — 5 input features):
  * a variational autoencoder trained on (mostly) normal windows;
  * features for the classifier = [reconstruction_error, z_mean_0, z_mean_1];
  * a logistic regression on those features -> probability in [0, 1],
    interpreted as the anomaly score (higher = more anomalous).

torch is imported lazily so the rest of the package (and the Go-facing
export path) works without it; tests use pytest.importorskip. If torch is
missing this module raises TorchUnavailable on fit()/load() — evaluate.py
treats that as "model C unavailable" rather than an error.

Retention rule (evaluate.py): model C is exported for runtime only when its
holdout ROC-AUC exceeds the best baseline (A or B) by >= RETAIN_MARGIN.
Otherwise the artifact is not written and the evaluation report records why.
"""
from __future__ import annotations

import json
import math
from typing import Any

from .features import FEATURE_NAMES

MODEL_TYPE = "vae_logistic"
RETAIN_MARGIN = 0.05  # required AUC improvement over best baseline to keep model C
LATENT_DIM = 2
HIDDEN_DIM = 16


class TorchUnavailable(RuntimeError):
    """torch is not installed; model C cannot be trained or loaded."""


def _torch():
    try:
        import torch  # noqa: PLC0415
        import torch.nn as nn  # noqa: PLC0415
    except ImportError as exc:  # pragma: no cover - environment dependent
        raise TorchUnavailable(
            "torch is not installed; install PyTorch to train/evaluate model C "
            "(optional dependency, see investigator/app/anomaly/README.md)"
        ) from exc
    return torch, nn


def build_classes():
    """Define VAE/encoder/decoder classes lazily (needs torch)."""
    torch, nn = _torch()

    class Encoder(nn.Module):
        def __init__(self, input_dim: int, hidden: int, latent: int):
            super().__init__()
            self.fc = nn.Linear(input_dim, hidden)
            self.mu = nn.Linear(hidden, latent)
            self.logvar = nn.Linear(hidden, latent)

        def forward(self, x):  # type: ignore[no-untyped-def]
            h = torch.relu(self.fc(x))
            return self.mu(h), self.logvar(h)

    class Decoder(nn.Module):
        def __init__(self, latent: int, hidden: int, output_dim: int):
            super().__init__()
            self.fc1 = nn.Linear(latent, hidden)
            self.fc2 = nn.Linear(hidden, output_dim)

        def forward(self, z):  # type: ignore[no-untyped-def]
            # sigmoid squashes to (0,1) so BCE reconstruction is valid and the
            # [0,1]-scaled inputs are a valid target distribution.
            return torch.sigmoid(self.fc2(torch.relu(self.fc1(z))))

    class VAE(nn.Module):
        def __init__(self, input_dim: int, hidden: int = HIDDEN_DIM, latent: int = LATENT_DIM):
            super().__init__()
            self.encoder = Encoder(input_dim, hidden, latent)
            self.decoder = Decoder(latent, hidden, input_dim)

        def forward(self, x):  # type: ignore[no-untyped-def]
            mu, logvar = self.encoder(x)
            if self.training:
                std = torch.exp(0.5 * logvar)
                eps = torch.randn_like(std)
                z = mu + eps * std
            else:
                # Deterministic inference: use the mean so scoring is
                # reproducible (the KL term still regularised mu at train time).
                z = mu
            return self.decoder(z), mu, logvar

    return VAE


def vae_loss(recon, x, mu, logvar):  # type: ignore[no-untyped-def]
    """Standard ELBO: BCE reconstruction (sum over features) + KL divergence."""
    torch, _ = _torch()
    bce = torch.nn.functional.binary_cross_entropy(recon, x, reduction="sum")
    kl = -0.5 * torch.sum(1 + logvar - mu.pow(2) - logvar.exp())
    return bce + kl


class VAEClassifierModel:
    """Container for the trained VAE + logistic classifier artifact."""

    def __init__(self, artifact: dict[str, Any]):
        if artifact.get("model_type") != MODEL_TYPE:
            raise ValueError(f"not a {MODEL_TYPE} artifact: {artifact.get('model_type')!r}")
        self.artifact = artifact
        self.model_version = artifact["model_version"]
        self.scaler_min = artifact["scaler"]["min"]
        self.scaler_max = artifact["scaler"]["max"]
        self.scaler_fill = artifact["scaler"]["fill"]
        self.vae_state = artifact["vae_state"]
        self.classifier = artifact["classifier"]  # {"coef": [[..]], "intercept": [..]}

    def score(self, values: dict[str, float | None]) -> tuple[float, list[str]]:
        """Return (score in [0,1], imputed feature names).

        Missing features are imputed with the training min-max midpoint
        (documented in the artifact as scaler.fill); imputed names are
        reported for provenance. Inputs are min-max scaled to [0,1] exactly
        as during training before the VAE forward pass.
        """
        torch, _ = _torch()
        row: list[float] = []
        imputed: list[str] = []
        for idx, name in enumerate(FEATURE_NAMES):
            value = values.get(name)
            if value is None:
                row.append(self.scaler_fill[idx])
                imputed.append(name)
            else:
                row.append(float(value))
        mins = self.scaler_min
        maxs = self.scaler_max
        scaled = [
            (row[idx] - mins[idx]) / (maxs[idx] - mins[idx]) if maxs[idx] > mins[idx] else 0.0
            for idx in range(len(FEATURE_NAMES))
        ]
        x = torch.tensor(scaled, dtype=torch.float32)

        VAE = build_classes()
        model = VAE(len(FEATURE_NAMES))
        model.load_state_dict(
            {k: torch.tensor(v) for k, v in self.vae_state.items()},
            strict=True,
        )
        model.eval()
        with torch.no_grad():
            recon, mu, _logvar = model(x.unsqueeze(0))
            recon_error = float(torch.mean((recon - x.unsqueeze(0)) ** 2))
        classifier_features = [recon_error, float(mu[0][0]), float(mu[0][1])]
        coef = self.classifier["coef"][0]
        intercept = self.classifier["intercept"][0]
        logit = intercept + sum(c * f for c, f in zip(coef, classifier_features))
        score = 1.0 / (1.0 + math.exp(-max(-60.0, min(60.0, logit))))
        return score, imputed


def fit(
    vectors: list[dict[str, float | None]],
    labels: list[int],
    *,
    model_version: str,
    epochs: int = 200,
    learning_rate: float = 1e-2,
    seed: int = 0,
) -> VAEClassifierModel:
    """Train VAE on all windows, then logistic regression on
    [recon_error, z_mean_0, z_mean_1] against 0/1 labels."""
    torch, _ = _torch()
    from sklearn.linear_model import LogisticRegression  # noqa: PLC0415

    if len(vectors) != len(labels):
        raise ValueError("vectors and labels must align")
    if not vectors:
        raise ValueError("no training vectors")
    torch.manual_seed(seed)

    # Min-max scaling to [0,1] (BCE target space). Missing features are
    # imputed with the per-feature midpoint of [min, max] — a neutral value
    # that neither looks minimal nor extreme. The same fill value is stored
    # in the artifact and used at inference time.
    mins: list[float] = []
    maxs: list[float] = []
    fills: list[float] = []
    columns: dict[str, list[float]] = {}
    for name in FEATURE_NAMES:
        observed = [v[name] for v in vectors if v.get(name) is not None]
        col_min = min(observed) if observed else 0.0
        col_max = max(observed) if observed else 1.0
        if col_max <= col_min:
            col_max = col_min + 1.0
        mins.append(col_min)
        maxs.append(col_max)
        fills.append((col_min + col_max) / 2.0)
        columns[name] = [
            float(v[name]) if v.get(name) is not None else (col_min + col_max) / 2.0
            for v in vectors
        ]

    matrix = torch.tensor(
        [
            [
                (columns[name][i] - mins[idx]) / (maxs[idx] - mins[idx])
                for idx, name in enumerate(FEATURE_NAMES)
            ]
            for i in range(len(vectors))
        ],
        dtype=torch.float32,
    )

    VAE = build_classes()
    model = VAE(len(FEATURE_NAMES))
    optimizer = torch.optim.Adam(model.parameters(), lr=learning_rate)
    model.train()
    for _ in range(epochs):
        optimizer.zero_grad()
        recon, mu, logvar = model(matrix)
        loss = vae_loss(recon, matrix, mu, logvar)
        loss.backward()
        optimizer.step()

    model.eval()
    with torch.no_grad():
        recon, mu, _logvar = model(matrix)
        recon_errors = torch.mean((recon - matrix) ** 2, dim=1).tolist()
        z_means = mu.tolist()

    classifier_x = [
        [recon_errors[i], z_means[i][0], z_means[i][1]] for i in range(len(vectors))
    ]
    if len(set(labels)) < 2:
        # Degenerate labels (all one class): use a constant classifier that
        # cannot separate — recorded honestly in the artifact.
        coef = [[0.0, 0.0, 0.0]]
        intercept = [0.0]
    else:
        clf = LogisticRegression(max_iter=1000, random_state=seed)
        clf.fit(classifier_x, labels)
        coef = clf.coef_.tolist()
        intercept = clf.intercept_.tolist()

    artifact = {
        "model_type": MODEL_TYPE,
        "model_version": model_version,
        "score_range": [0.0, 1.0],
        "score_semantics": "logistic P(anomaly | recon_error, z_mean); higher = more anomalous",
        "feature_names": FEATURE_NAMES,
        "scaler": {"min": mins, "max": maxs, "fill": fills},
        "vae_state": {k: t.tolist() for k, t in model.state_dict().items()},
        "classifier": {"coef": coef, "intercept": intercept},
        "trained_on": {
            "n_vectors": len(vectors),
            "n_positive": int(sum(labels)),
            "epochs": epochs,
            "seed": seed,
        },
    }
    return VAEClassifierModel(artifact)


def save(path: str, model: VAEClassifierModel) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(model.artifact, fh, indent=2, sort_keys=True)
        fh.write("\n")


def load(path: str) -> VAEClassifierModel:
    with open(path, encoding="utf-8") as fh:
        return VAEClassifierModel(json.load(fh))
