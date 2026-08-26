# Two docs

```
export TMPDIR=$(mktemp -d)
python -m venv ${TMPDIR}/venv
python3 -m venv "${TMPDIR}/venv"
source "${TMPDIR}/venv/bin/activate"
python3.14 -m pip install --upgrade pip
pip3.14 install -r requirements.txt
sphinx-autobuild . "${TMPDIR}/build"
```
