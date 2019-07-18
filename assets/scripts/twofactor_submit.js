/* global Headers, fetch */
;(function(window, document) {
  if (!window.fetch || !window.Headers || !window.FormData) return

  const loginForm = document.getElementById('login-form')
  const loginField = document.getElementById('login-field')
  const redirectInput = document.getElementById('redirect')
  const submitButton = document.getElementById('login-submit')
  const twoFactorPasscodeInput = document.getElementById('two-factor-passcode')
  const twoFactorTokenInput = document.getElementById('two-factor-token')
  const twoFactorTrustDeviceCheckbox = document.getElementById(
    'two-factor-trust-device'
  )
  const longRunSessionCheckbox = document.getElementById('long-run-session')
  const csrfTokenInput = document.getElementById('csrf_token')

  let errorPanel = loginForm && loginForm.querySelector('.wizard-errors')

  const twoFactorTrustedDeviceTokenKey = 'two-factor-trusted-device-token'
  let localStorage = null
  try {
    localStorage = window.localStorage
  } catch (e) {
    // do nothing
  }

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

  const onSubmitTwoFactorCode = function(event) {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const longRunSession =
      longRunSessionCheckbox && longRunSessionCheckbox.checked ? '1' : '0'
    const passcode = twoFactorPasscodeInput.value
    const token = twoFactorTokenInput.value
    const trustDevice = twoFactorTrustDeviceCheckbox.checked ? '1' : '0'
    const redirect = redirectInput.value + window.location.hash

    const headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')

    const reqBody =
      'two-factor-passcode=' +
      encodeURIComponent(passcode) +
      '&long-run-session=' +
      encodeURIComponent(longRunSession) +
      '&two-factor-token=' +
      encodeURIComponent(token) +
      '&two-factor-generate-trusted-device-token=' +
      encodeURIComponent(trustDevice) +
      '&redirect=' +
      encodeURIComponent(redirect) +
      '&csrf_token=' +
      encodeURIComponent(csrfTokenInput.value)
    fetch('/auth/twofactor', {
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
              if (
                localStorage &&
                typeof body.two_factor_trusted_device_token == 'string'
              ) {
                localStorage.setItem(
                  twoFactorTrustedDeviceTokenKey,
                  body.two_factor_trusted_device_token
                )
              }
              if (body.redirect) {
                window.location = body.redirect
              }
            } else {
              showError(body.error)
              twoFactorPasscodeInput.classList.add('is-error')
              twoFactorPasscodeInput.select()
            }
          })
          .catch(showError)
      })
      .catch(showError)
  }

  // Responsive design
  if (document.body.clientWidth > 1024) {
    const avatars = document.getElementsByClassName('c-avatar')
    for (const avatar of avatars) {
      avatar.classList.add('c-avatar--xlarge')
    }
  }

  loginForm.addEventListener('submit', onSubmitTwoFactorCode)
})(window, document)
