;(function(window, document) {
  if (!window.fetch || !window.Headers || !window.FormData) return

  const loginForm = document.getElementById('login-form')
  const resetForm = document.getElementById('renew-passphrase-form')

  const passphraseInput = document.getElementById('password')
  const submitButton = document.getElementById('login-submit')

  const twoFactorTrustedDeviceTokenKey = 'two-factor-trusted-device-token'
  const twoFactorTrustedDomainInput = document.getElementById(
    'two-factor-trusted-device-token'
  )

  const longRunSessionCheckbox = document.getElementById('long-run-session')

  let localStorage = null
  try {
    localStorage = window.localStorage
  } catch (e) {
    // do nothing
  }

  // Set the trusted device token from the localstorage in the form if it exists
  const twoFactorTrustedDeviceToken =
    (localStorage && localStorage.getItem(twoFactorTrustedDeviceTokenKey)) || ''

  if (loginForm) {
    longRunSessionCheckbox.value = longRunSessionCheckbox.checked ? '1' : '0'
    twoFactorTrustedDomainInput.value = twoFactorTrustedDeviceToken
  }

  let errorPanel = loginForm && loginForm.querySelector('.wizard-errors')
  const loginField = document.getElementById('login-field')
  const showError = function(error) {
    if (error) {
      error = '' + error
    } else {
      error = 'The Cozy server is unavailable. Do you have network?'
    }

    if (!errorPanel) {
      errorPanel = document.createElement('p')
      errorPanel.classList.add('wizard-errors', 'u-error')
      loginField.insertBefore(errorPanel, loginField.firstChild)
    }

    errorPanel.textContent = error
    submitButton.removeAttribute('disabled')
  }

  const onSubmitPassphrase = function(event) {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const redirectInput = document.getElementById('redirect')
    const passphrase = passphraseInput.value
    const longRunSession = longRunSessionCheckbox.checked ? '1' : '0'
    const redirect = redirectInput.value + window.location.hash
    const csrfTokenInput = document.getElementById('csrf_token')

    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')

    const reqBody =
      'passphrase=' +
      encodeURIComponent(passphrase) +
      '&two-factor-trusted-device-token=' +
      encodeURIComponent(twoFactorTrustedDeviceToken) +
      '&long-run-session=' +
      encodeURIComponent(longRunSession) +
      '&redirect=' +
      encodeURIComponent(redirect) +
      '&csrf_token=' +
      encodeURIComponent(csrfTokenInput.value)

    fetch('/auth/login', {
      method: 'POST',
      headers: headers,
      body: reqBody,
      credentials: 'same-origin'
    })
      .then(function(response) {
        const loginSuccess = response.status < 400
        response
          .json()
          .then(function(body) {
            if (loginSuccess) {
              submitButton.childNodes[1].innerHTML =
                '<svg width="16" height="16"><use xlink:href="#fa-check"/></svg>'
              submitButton.classList.add('c-btn--highlight')
              if (body.redirect) {
                window.location = body.redirect
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
  loginForm && loginForm.addEventListener('submit', onSubmitPassphrase)

  // Used for passphrase reset
  resetForm &&
    resetForm.addEventListener('submit', function(event) {
      event.preventDefault()
      const label = window.password.getStrength(passphraseInput.value).label
      if (label == 'weak') {
        return false
      } else {
        resetForm.submit()
      }
    })

  resetForm &&
    passphraseInput.addEventListener('input', function(event) {
      const label = window.password.getStrength(event.target.value).label
      submitButton[label == 'weak' ? 'setAttribute' : 'removeAttribute'](
        'disabled',
        ''
      )
    })

  passphraseInput.focus()
  loginForm && submitButton.removeAttribute('disabled')

  // Responsive design
  if (document.body.clientWidth > 1024) {
    const avatars = document.getElementsByClassName('c-avatar')
    for (const avatar of avatars) {
      avatar.classList.add('c-avatar--xlarge')
    }
  }
})(window, document)
