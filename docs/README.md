# Documentation `two`

Construction locale, avec rechargement automatique :

```bash
TMPDIR=$(mktemp -d)
python3 -m venv "${TMPDIR}/venv"
source "${TMPDIR}/venv/bin/activate"
pip install --upgrade pip
pip install -r requirements.txt
sphinx-autobuild . "${TMPDIR}/build"
```

Construction simple, avec les mêmes options que la CI (avertissements bloquants) :

```bash
sphinx-build -b html -W --keep-going . _build/html
```

La référence de l'API est générée depuis `../api/agent.yaml` : c'est la source unique du
contrat, la documentation ne la recopie pas.

## Schémas

Les schémas simples sont en mermaid, directement dans les pages. Les schémas travaillés sont des
SVG dans `schemas/`, en deux variantes :

```
schemas/<nom>.svg        thème clair   → .. figure:: /schemas/<nom>.svg  :class: only-light
schemas/<nom>-dark.svg   thème sombre  → .. figure:: /schemas/<nom>-dark.svg :class: only-dark
```

`schemas/` est dans `exclude_patterns` : Sphinx n'y cherche pas de pages, mais copie les fichiers
référencés par une directive `figure` ou `image`.

Les schémas d'architecture (`architecture-ensemble` et `architecture-cluster`) sont générés : ne
pas éditer leurs SVG, modifier `schemas/gen_ensemble.py` ou `schemas/gen_cluster.py` (même style,
le second réutilise les fonctions du premier), puis régénérer les deux variantes :

```bash
python3 -B schemas/gen_ensemble.py schemas
python3 -B schemas/gen_cluster.py schemas
```

Un export draw.io se dépose tel quel sous ce nom. Exporter en **SVG éditable** (« Include a copy
of my diagram ») pour pouvoir rouvrir le fichier dans draw.io ensuite.
