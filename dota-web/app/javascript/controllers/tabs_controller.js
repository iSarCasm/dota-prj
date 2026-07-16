import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static targets = ["tab", "panel"]
  static values = { index: { type: Number, default: 0 } }

  connect() {
    this.show(this.indexValue)
  }

  select(event) {
    const index = this.tabTargets.indexOf(event.currentTarget)
    if (index < 0) return
    this.indexValue = index
    this.show(index)
  }

  show(index) {
    this.tabTargets.forEach((tab, i) => {
      const selected = i === index
      tab.classList.toggle("is-active", selected)
      tab.setAttribute("aria-selected", selected ? "true" : "false")
      tab.tabIndex = selected ? 0 : -1
    })

    this.panelTargets.forEach((panel, i) => {
      const selected = i === index
      panel.classList.toggle("is-active", selected)
      panel.setAttribute("aria-hidden", selected ? "false" : "true")
      panel.inert = !selected
    })
  }
}
