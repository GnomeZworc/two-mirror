# Configuration file for the Sphinx documentation builder.
#
# For the full list of built-in configuration values, see the documentation:
# https://www.sphinx-doc.org/en/master/usage/configuration.html

# -- Project information -----------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#project-information

project = 'two'
copyright = '2026, Nicolas Boufidjeline'
author = 'Nicolas Boufidjeline'
version = '0.1'
release = '0.1.0'


# -- General configuration ---------------------------------------------------

templates_path = ['_templates']
exclude_patterns = ['_build', 'README.md', 'requirements.txt', 'schemas']

language = 'fr'

extensions = [
    'myst_parser',
    'sphinxcontrib.mermaid',
    'sphinxcontrib.openapi',
]

myst_enable_extensions = [
    'colon_fence',
    'deflist',
]

source_suffix = {
    '.rst': 'restructuredtext',
    '.md': 'markdown',
}

# -- Options for HTML output -------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#options-for-html-output

html_theme = 'sphinx_book_theme'
html_static_path = []
html_show_sphinx = False

html_theme_options = {
    'home_page_in_toc': True,
}
