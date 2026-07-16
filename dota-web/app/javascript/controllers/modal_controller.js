import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static targets = ["dialog"]

  open() {
    this.dialogTarget.showModal()
  }

  close() {
    this.dialogTarget.close()
  }

  // Clicks on the dialog element itself land on the backdrop area
  backdropClose(event) {
    if (event.target === this.dialogTarget) this.close()
  }
}
