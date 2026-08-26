# Configuration file for the Sphinx documentation builder.
#
# For the full list of built-in configuration values, see the documentation:
# https://www.sphinx-doc.org/en/master/usage/configuration.html

# -- Project information -----------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#project-information

project = 'Two doc\'s'
copyright = '2023, Nicolas Boufidjeline'
author = 'Nicolas Boufidjeline'


# -- General configuration ---------------------------------------------------

templates_path = ['_templates']
exclude_patterns = []

language = 'fr'

# -- Options for HTML output -------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#options-for-html-output

html_theme = 'sphinx_book_theme'
html_static_path = []

extensions = [
    'myst_parser',
    'sphinxcontrib.mermaid'
]

html_show_sphinx = False

source_suffix = {
    '.rst': 'restructuredtext',
}
