;(function (w, d) {
  if (!w.fetch || !w.Headers) return

  const form = d.getElementById('confirm-flagship-form')
  const field = d.getElementById('code-field')
  const submitButton = d.getElementById('confirm-flagship-submit')
  const codeInput = d.getElementById('code-input')
  const tokenInput = d.getElementById('confirm-token')

  const onSubmitCode = function (event) {
    event.preventDefault()
    codeInput.setAttribute('disabled', true)
    submitButton.setAttribute('disabled', true)

    const data = new URLSearchParams()
    data.append('code', codeInput.value)
    data.append('token', tokenInput.value)

    const headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')

    return fetch(form.action, {
      method: 'POST',
      headers: headers,
      body: data,
      credentials: 'same-origin',
    })
      .then((response) => {
        if (response.status < 400) {
          const tooltip = field.querySelector('.invalid-tooltip')
          if (tooltip) {
            tooltip.classList.add('d-none')
          }
          submitButton.innerHTML = '<span class="icon icon-check"></span>'
          submitButton.classList.add('btn-done')
          w.location.reload()
        } else {
          return response.json().then(function (body) {
            w.showError(field, body.error)
          })
        }
      })
      .catch((err) => w.showError(field, err))
  }

  form.addEventListener('submit', onSubmitCode)
})(window, document)
