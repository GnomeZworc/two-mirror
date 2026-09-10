# Configuration file for the Sphinx documentation builder.
#
# For the full list of built-in configuration values, see the documentation:
# https://www.sphinx-doc.org/en/master/usage/configuration.html

import os

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

# Le thème publie le source de chaque page dans _sources/ et l'expose derrière
# un bouton de téléchargement. Les deux vont ensemble : couper la copie sans
# couper le bouton laisserait un lien mort vers un répertoire vide.
html_copy_source = False
html_show_sourcelink = False

# Le sélecteur de version est piloté par le workflow de publication : hors CI
# la variable est absente, le sélecteur n'apparaît pas, et le build ne dépend
# d'aucun réseau.
_docs_version = os.environ.get('DOCS_VERSION')

html_theme_options = {
    'home_page_in_toc': True,
    'use_download_button': False,
    'icon_links': [
        {
            'name': 'Dépôt',
            'url': 'https://git.g3e.fr/syonad/two',
            'icon': 'fa-solid fa-code-branch',
            'type': 'fontawesome',
        },
    ],
}

if _docs_version:
    html_theme_options['switcher'] = {
        # Chemin relatif volontairement : le thème le résout contre la racine
        # de la version courante, donc toujours dans la même origine que la
        # page. Une URL absolue ferait échouer la requête en CORS dès que le
        # site est consulté depuis un autre hôte — un serveur de test local,
        # par exemple.
        'json_url': '../switcher.json',
        'version_match': _docs_version,
    }
    # Le thème book vide navbar_start et place tout dans la barre latérale : le
    # sélecteur doit donc y être inséré explicitement, à côté du logo.
    html_sidebars = {
        '**': [
            'navbar-logo.html',
            'icon-links.html',
            'version-switcher.html',
            'search-button-field.html',
            'sbt-sidebar-nav.html',
        ]
    }
