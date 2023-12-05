(function (w, d) {
  if (!w.fetch || !w.Headers) return

  const form = d.getElementById('share-by-link-password-form')
  const field = d.getElementById('password-field')
  const submit = d.getElementById('password-submit')
  const input = d.getElementById('password')
  const permID = d.getElementById('perm-id')

  const onSubmit = function (event) {
    event.preventDefault()
    input.setAttribute('disabled', true)
    submit.setAttribute('disabled', true)

    const data = new URLSearchParams()
    data.append('password', input.value)
    data.append('perm_id', permID.value)

    const headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')

    return fetch(form.action, {
      method: 'POST',
      headers: headers,
      body: data,
      credentials: 'include',
    })
      .then((response) => {
        if (response.status < 400) {
          const tooltip = field.querySelector('.invalid-tooltip')
          if (tooltip) {
            tooltip.classList.add('d-none')
          }
          submit.innerHTML = '<span class="icon icon-check"></span>'
          submit.classList.add('btn-done')
          w.location.reload()
        } else {
          return response.json().then(function (body) {
            w.showError(field, body.error)
            input.removeAttribute('disabled')
            submit.removeAttribute('disabled')
          })
        }
      })
      .catch((err) => w.showError(field, err))
  }

  form.addEventListener('submit', onSubmit)
})(window, document)
