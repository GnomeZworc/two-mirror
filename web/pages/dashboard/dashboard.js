import { SidePanel } from '../../components/side-panel/side-panel.js'

export function init(main) {
  main.querySelectorAll('[data-panel-title]').forEach(btn => {
    btn.addEventListener('click', () => {
      SidePanel.open({
        title: btn.dataset.panelTitle,
        width: btn.dataset.panelWidth ?? '480px',
        content: `<p style="color:#a6adc8;font-size:14px;line-height:1.6">
          Contenu du panneau <strong style="color:#cdd6f4">${btn.dataset.panelTitle}</strong>.<br>
          Largeur : ${btn.dataset.panelWidth}.<br><br>
          Tu peux ouvrir plusieurs panneaux — ils s'empilent vers la gauche.
          Ferme avec ✕, Échap, ou en cliquant le fond.
        </p>`,
      })
    })
  })

  main.querySelectorAll('[data-action="create-vm"]').forEach(btn => {
    btn.addEventListener('click', () => {
      SidePanel.open({
        title: 'Nouvelle VM',
        width: '520px',
        content: `<form-vm></form-vm>`,
      })
      // form-vm gère son propre submit et dispatche vm-saved — pas besoin de querySelector
    })
  })

  main.querySelectorAll('[data-action="edit-vm-i-test1"]').forEach(btn => {
    btn.addEventListener('click', () => {
      SidePanel.open({
        title: 'Modifier i-test1',
        width: '520px',
        content: `<form-vm vm-name="i-test1"></form-vm>`
      })
      // form-vm gère son propre submit et dispatche vm-saved — pas besoin de querySelector
    })
  })

  // vm-saved est attaché sur main (pas document) → nettoyé automatiquement
  // quand le router remplace le contenu de <main>
  main.addEventListener('vm-saved', e => {
    console.log('vm-saved', e.detail)
  })
}
