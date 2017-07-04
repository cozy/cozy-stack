/* global Headers, fetch */
(function (w) {
  if (!w.fetch || !w.Headers || !w.FormData) return

  const d = window.document
  let form = d.getElementById('login-form')
  const url = form && form.getAttribute('action')
  const passphraseInput = d.getElementById('password')
  const redirectInput = d.getElementById('redirect')
  const submitButton = d.getElementById('login-submit')
  let errorPanel = form && form.querySelector('.errors')

  const showError = function (error) {
    error = error || 'The Cozy server is unavailable. Do you have network?'

    if (!errorPanel) {
      errorPanel = d.createElement('div')
      errorPanel.classList.add('errors')
      form.appendChild(errorPanel)
    }

    errorPanel.innerHTML = '<p>' + error + '</p>'
    submitButton.removeAttribute('disabled')
  }

  form && form.addEventListener('submit', (event) => {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const passphrase = passphraseInput.value
    const redirect = redirectInput.value + window.location.hash
    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')
    fetch('/auth/login', {
      method: 'POST',
      headers: headers,
      body: 'passphrase=' + encodeURIComponent(passphrase) + '&redirect=' + encodeURIComponent(redirect),
      credentials: 'same-origin'
    }).then((response) => {
      const loginSuccess = response.status < 400
      response.json().then((body) => {
        if (loginSuccess) {
          submitButton.innerHTML = '<svg width="16" height="16"><use xlink:href="#fa-check"/></svg>'
          submitButton.classList.add('btn-success')
          if (body.redirect) {
            window.location = body.redirect
          } else {
            form.submit()
          }
        } else {
          showError(body.error)
        }
      }).catch(showError)
    }).catch(showError)
  })

  passphraseInput.focus()
  if (!/passphrase_renew$/.test(document.location.pathname)) {
    submitButton.removeAttribute('disabled')
  }
})(window)
