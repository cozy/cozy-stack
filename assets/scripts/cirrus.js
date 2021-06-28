;(function (w, d) {
  // On android, change the address bar color to match the page background
  const paperColor = getComputedStyle(d.body).getPropertyValue(
    '--paperBackgroundColor'
  )
  if (paperColor) {
    const themeColor = d.querySelector('meta[name=theme-color]')
    themeColor.setAttribute('content', paperColor)
  }

  // Add an helper function to display an error on a form input
  w.showError = (field, message) => {
    let tooltip = field.querySelector('.invalid-tooltip')
    let input = field.querySelector('input')
    let submit = input.form.querySelector('[type=submit]')
    let error = 'The Cozy server is unavailable. Do you have network?'
    if (message) {
      error = '' + message
    }

    if (tooltip) {
      tooltip.lastChild.textContent = error
    } else {
      tooltip = d.createElement('div')
      tooltip.classList.add('invalid-tooltip', 'mb-1')
      const arrow = d.createElement('div')
      arrow.classList.add('tooltip-arrow')
      tooltip.appendChild(arrow)
      const icon = d.createElement('span')
      icon.classList.add('icon', 'icon-alert', 'bg-danger')
      tooltip.appendChild(icon)
      tooltip.append(error)
      field.appendChild(tooltip)
    }

    submit.removeAttribute('disabled')
    input.removeAttribute('disabled')
    input.classList.add('is-invalid')
    input.select()
  }

  // Use the browser history for managing cancel links
  const cancel = w.document.querySelector('a.cancel')
  if (cancel) {
    cancel.addEventListener('click', function (event) {
      if (w.history.length > 1) {
        event.preventDefault()
        w.history.back()
      }
    })
  }

  const expand = w.document.querySelector('a.expand')
  if (expand) {
    expand.addEventListener('click', function (event) {
      event.preventDefault()
      expand.classList.toggle('expanded')
    })
  }
})(window, document)
