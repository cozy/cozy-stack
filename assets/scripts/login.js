;(function (w, d) {
  if (!w.fetch || !w.Headers) return

  const loginForm = d.getElementById('login-form')
  const passphraseInput = d.getElementById('password')
  const submitButton = d.getElementById('login-submit')
  const redirectInput = d.getElementById('redirect')
  const csrfTokenInput = d.getElementById('csrf_token')
  const stateInput = d.getElementById('state')
  const clientIdInput = d.getElementById('client_id')
  const loginField = d.getElementById('login-field')
  const longRunSessionCheckbox = d.getElementById('long-run-session')
  const twoFactorTrustedDomainInput = d.getElementById('trusted-device-token')

  // Set the trusted device token from the localstorage in the form if it exists
  try {
    const storage = w.localStorage
    const twoFactorTrustedDeviceToken =
      storage.getItem('trusted-device-token') || ''
    twoFactorTrustedDomainInput.value = twoFactorTrustedDeviceToken
  } catch (e) {
    // do nothing
  }

  let errorPanel = loginForm.querySelector('.wizard-errors')
  const showError = function (message) {
    if (!errorPanel) {
      errorPanel = d.createElement('p')
      errorPanel.classList.add('wizard-errors', 'u-error')
      loginField.insertBefore(errorPanel, loginField.firstChild)
    }

    let error = 'The Cozy server is unavailable. Do you have network?'
    if (message) {
      error = '' + message
    }
    errorPanel.textContent = error
    submitButton.removeAttribute('disabled')
  }

  const onSubmitPassphrase = function (event) {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const passphrase = passphraseInput.value
    const longRunSession =
      longRunSessionCheckbox && longRunSessionCheckbox.checked ? '1' : '0'
    const redirect = redirectInput && redirectInput.value + w.location.hash

    let passPromise = Promise.resolve(passphrase)
    const salt = loginForm.dataset.salt
    const iterations = parseInt(loginForm.dataset.iterations, 10)
    if (iterations > 0) {
      passPromise = w.password
        .hash(passphrase, salt, iterations)
        .then(({ hashed }) => hashed)
    }

    passPromise
      .then((pass) => {
        let reqBody =
          'passphrase=' +
          encodeURIComponent(pass) +
          '&trusted-device-token=' +
          encodeURIComponent(twoFactorTrustedDomainInput.value) +
          '&long-run-session=' +
          encodeURIComponent(longRunSession) +
          '&redirect=' +
          encodeURIComponent(redirect) +
          '&csrf_token=' +
          encodeURIComponent(csrfTokenInput.value)

        // For the /auth/authorize/move && /auth/confirm pages
        if (stateInput) {
          reqBody += '&state=' + encodeURIComponent(stateInput.value)
        }
        if (clientIdInput) {
          reqBody += '&client_id=' + encodeURIComponent(clientIdInput.value)
        }

        // TODO use a JSON body
        let headers = new Headers()
        headers.append('Content-Type', 'application/x-www-form-urlencoded')
        headers.append('Accept', 'application/json')
        return fetch(loginForm.action, {
          method: 'POST',
          headers: headers,
          body: reqBody,
          credentials: 'same-origin',
        })
      })
      .then((response) => {
        return response.json().then((body) => {
          if (response.status < 400) {
            submitButton.childNodes[1].innerHTML =
              '<svg width="16" height="16"><use xlink:href="#fa-check"/></svg>'
            submitButton.classList.add('c-btn--highlight')
            if (body.redirect) {
              w.location = body.redirect
            }
          } else {
            showError(body.error)
            passphraseInput.classList.add('is-error')
            passphraseInput.select()
          }
        })
      })
      .catch(showError)
  }

  loginForm.addEventListener('submit', onSubmitPassphrase)
  passphraseInput.focus()
  submitButton.removeAttribute('disabled')
})(window, document)
