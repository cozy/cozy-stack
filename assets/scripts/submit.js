/* global Headers, fetch */
(function (window, document) {
  if (!window.fetch || !window.Headers || !window.FormData) return

  const loginForm = document.getElementById('login-form')
  const resetForm = document.getElementById('renew-passphrase-form')

  const url = loginForm && loginForm.getAttribute('action')

  const passphraseInput = document.getElementById('password')
  const redirectInput = document.getElementById('redirect')
  const submitButton = document.getElementById('login-submit')

  let errorPanel = loginForm && loginForm.querySelector('.errors')

  const showError = function (error) {
    error = error || 'The Cozy server is unavailable. Do you have network?'

    if (!errorPanel) {
      errorPanel = document.createElement('div')
      errorPanel.classList.add('errors')
      loginForm.appendChild(errorPanel)
    }

    errorPanel.innerHTML = '<p>' + error + '</p>'
    submitButton.removeAttribute('disabled')
  }

  loginForm && loginForm.addEventListener('submit', (event) => {
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
          passphraseInput.select()
        }
      }).catch(showError)
    }).catch(showError)
  })

  resetForm && resetForm.addEventListener('submit', (event) => {
    event.preventDefault()
    const { label } = window.password.getStrength(passphraseInput.value)
    if (label == 'weak') {
      return false
    } else {
      resetForm.submit()
    }
  })

  resetForm && passphraseInput.addEventListener('input', function(event) {
    const { label } = window.password.getStrength(event.target.value)
    submitButton[label == 'weak' ? 'setAttribute' : 'removeAttribute']('disabled', '')
  })

  passphraseInput.focus()
  loginForm && submitButton.removeAttribute('disabled')
})(window, document)
