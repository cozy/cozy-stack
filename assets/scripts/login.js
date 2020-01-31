;(function(w, d) {
  if (!w.fetch || !w.Headers) return

  const loginForm = d.getElementById('login-form')
  const passphraseInput = d.getElementById('password')
  const submitButton = d.getElementById('login-submit')

  // Set the trusted device token from the localstorage in the form if it exists
  let storage = null
  try {
    storage = w.localStorage
  } catch (e) {
    // do nothing
  }
  const twoFactorTrustedDomainInput = d.getElementById(
    'two-factor-trusted-device-token'
  )
  const twoFactorTrustedDeviceToken =
    (storage && storage.getItem('two-factor-trusted-device-token')) || ''
  twoFactorTrustedDomainInput.value = twoFactorTrustedDeviceToken

  const longRunSessionCheckbox = d.getElementById('long-run-session')
  longRunSessionCheckbox.value = longRunSessionCheckbox.checked ? '1' : '0'

  let errorPanel = loginForm.querySelector('.wizard-errors')
  const loginField = d.getElementById('login-field')
  const showError = function(error) {
    if (error) {
      error = '' + error
    } else {
      error = 'The Cozy server is unavailable. Do you have network?'
    }

    if (!errorPanel) {
      errorPanel = d.createElement('p')
      errorPanel.classList.add('wizard-errors', 'u-error')
      loginField.insertBefore(errorPanel, loginField.firstChild)
    }

    errorPanel.textContent = error
    submitButton.removeAttribute('disabled')
  }

  const onSubmitPassphrase = function(event) {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const passphrase = passphraseInput.value
    const redirectInput = d.getElementById('redirect')
    const longRunSession = longRunSessionCheckbox.checked ? '1' : '0'
    const redirect = redirectInput.value + w.location.hash
    const csrfTokenInput = d.getElementById('csrf_token')

    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')

    let passPromise = Promise.resolve(passphrase)
    const salt = loginForm.dataset.salt
    const iterations = parseInt(loginForm.dataset.iterations, 10)
    if (iterations > 0) {
      passPromise = w.password
        .hash(passphrase, salt, iterations)
        .then(({ hashed }) => hashed)
    }

    passPromise
      .then(pass => {
        const reqBody =
          'passphrase=' +
          encodeURIComponent(pass) +
          '&two-factor-trusted-device-token=' +
          encodeURIComponent(twoFactorTrustedDeviceToken) +
          '&long-run-session=' +
          encodeURIComponent(longRunSession) +
          '&redirect=' +
          encodeURIComponent(redirect) +
          '&csrf_token=' +
          encodeURIComponent(csrfTokenInput.value)

        return fetch('/auth/login', {
          method: 'POST',
          headers: headers,
          body: reqBody,
          credentials: 'same-origin'
        })
      })
      .then(response => {
        const loginSuccess = response.status < 400
        response
          .json()
          .then(body => {
            if (loginSuccess) {
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
          .catch(showError)
      })
      .catch(showError)
  }

  loginForm.addEventListener('submit', onSubmitPassphrase)
  passphraseInput.focus()
  submitButton.removeAttribute('disabled')
})(window, document)
