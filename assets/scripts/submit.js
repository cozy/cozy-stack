/* global Headers, fetch */
(function (window, document) {
  if (!window.fetch || !window.Headers || !window.FormData) return

  const loginForm = document.getElementById('login-form')
  const loginField = document.getElementById('login-field')
  const resetForm = document.getElementById('renew-passphrase-form')

  const url = loginForm && loginForm.getAttribute('action')

  const passphraseInput = document.getElementById('password')
  const redirectInput = document.getElementById('redirect')
  const submitButton = document.getElementById('login-submit')
  const twoFactorPasscodeInput = document.getElementById('two-factor-passcode')
  const twoFactorTokenInput = document.getElementById('two-factor-token')
  const twoFactorTrustDeviceCheckbox = document.getElementById('two-factor-trust-device')
  const longRunSessionCheckbox = document.getElementById('long-run-session')
  const twoFactorForms = document.getElementsByClassName('two-factor-form')
  const passwordForms = document.getElementsByClassName('password-form')
  const csrfTokenInput = document.getElementById('csrf_token')

  let errorPanel = loginForm && loginForm.querySelector('.wizard-errors')

  const twoFactorTrustedDeviceTokenKey = 'two-factor-trusted-device-token'
  let localStorage = null
  try {
    localStorage = window.localStorage
  } catch(e) {}

  const showError = function (error) {
    if (error) {
      error = '' + error
    } else {
      error = 'The Cozy server is unavailable. Do you have network?'
    }

    if (!errorPanel) {
      errorPanel = document.createElement('p')
      errorPanel.classList.add('wizard-errors', 'u-error')
      loginField.insertBefore(errorPanel, loginField.firstChild);
    }

    errorPanel.textContent = error;
    submitButton.removeAttribute('disabled')
  }

  const onSubmitPassphrase = function(event) {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const passphrase = passphraseInput.value
    const longRunSession = longRunSessionCheckbox.checked ? '1' : '0'
    const twoFactorTrustedDeviceToken = (localStorage && localStorage.getItem(twoFactorTrustedDeviceTokenKey)) || ''
    const redirect = redirectInput.value + window.location.hash
    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')
    const reqBody = 'passphrase=' + encodeURIComponent(passphrase) +
      '&two-factor-trusted-device-token=' + encodeURIComponent(twoFactorTrustedDeviceToken) +
      '&long-run-session=' + encodeURIComponent(longRunSession) +
      '&redirect=' + encodeURIComponent(redirect) +
      '&csrf_token=' + encodeURIComponent(csrfTokenInput.value);
    fetch('/auth/login', {
      method: 'POST',
      headers: headers,
      body: reqBody,
      credentials: 'same-origin'
    }).then(function(response) {
      const loginSuccess = response.status < 400
      response.json().then(function(body) {
        if (loginSuccess) {
          if (body.two_factor_token) {
            renderTwoFactorForm(body.two_factor_token)
            return
          }
          submitButton.childNodes[1].innerHTML = '<svg width="16" height="16"><use xlink:href="#fa-check"/></svg>'
          submitButton.classList.add('c-btn--highlight')
          if (body.redirect) {
            window.location = body.redirect
          } else {
            form.submit()
          }
        } else {
          showError(body.error)
          passphraseInput.classList.add('is-error')
          passphraseInput.select()
        }
      }).catch(showError)
    }).catch(showError)
  }

  const onSubmitTwoFactorCode = function(event) {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const longRunSession = longRunSessionCheckbox && longRunSessionCheckbox.checked ? '1' : '0'
    const passcode = twoFactorPasscodeInput.value
    const token = twoFactorTokenInput.value
    const trustDevice = twoFactorTrustDeviceCheckbox.checked ? '1' : '0'
    const redirect = redirectInput.value + window.location.hash

    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')
    const reqBody = 'two-factor-passcode=' + encodeURIComponent(passcode) +
      '&long-run-session=' + encodeURIComponent(longRunSession) +
      '&two-factor-token=' + encodeURIComponent(token) +
      '&two-factor-generate-trusted-device-token=' + encodeURIComponent(trustDevice) +
      '&redirect=' + encodeURIComponent(redirect) +
      '&csrf_token=' + encodeURIComponent(csrfTokenInput.value);
    fetch('/auth/login', {
      method: 'POST',
      headers: headers,
      body: reqBody,
      credentials: 'same-origin'
    }).then(function(response) {
      const loginSuccess = response.status < 400
      response.json().then(function(body) {
        if (loginSuccess) {
          submitButton.childNodes[3].innerHTML = '<svg width="16" height="16"><use xlink:href="#fa-check"/></svg>'
          submitButton.classList.add('c-btn--highlight')
          if (localStorage && typeof body.two_factor_trusted_device_token == 'string') {
            localStorage.setItem(twoFactorTrustedDeviceTokenKey, body.two_factor_trusted_device_token)
          }
          if (body.redirect) {
            window.location = body.redirect
          } else {
            form.submit()
          }
        } else {
          showError(body.error)
          twoFactorPasscodeInput.classList.add('is-error')
          twoFactorPasscodeInput.select()
        }
      }).catch(showError)
    }).catch(showError)
  }

  function renderTwoFactorForm(twoFactorToken) {
    for (let i = 0; i < twoFactorForms.length; i++) {
      twoFactorForms[i].classList.remove('u-hide')
    }
    for (let i = 0; i < passwordForms.length; i++) {
      passwordForms[i].classList.add('u-hide')
    }
    if (errorPanel) errorPanel.textContent = ''
    submitButton.removeAttribute('disabled')
    twoFactorTokenInput.value = twoFactorToken
    twoFactorPasscodeInput.value = ''
    twoFactorPasscodeInput.focus()
    loginForm.removeEventListener('submit', onSubmitPassphrase)
    loginForm.addEventListener('submit', onSubmitTwoFactorCode)
  }

  loginForm && loginForm.addEventListener('submit', onSubmitPassphrase)

  resetForm && resetForm.addEventListener('submit', function(event) {
    event.preventDefault()
    const label = window.password.getStrength(passphraseInput.value).label
    if (label == 'weak') {
      return false
    } else {
      resetForm.submit()
    }
  })

  resetForm && passphraseInput.addEventListener('input', function(event) {
    const label = window.password.getStrength(event.target.value).label
    submitButton[label == 'weak' ? 'setAttribute' : 'removeAttribute']('disabled', '')
  })

  passphraseInput.focus()
  loginForm && submitButton.removeAttribute('disabled')

  // Responsive design
  if (document.body.clientWidth > 1024) {
    const avatars = document.getElementsByClassName('c-avatar')
    for (const avatar of avatars) {
      avatar.classList.add('c-avatar--xlarge', 'u-mv-1')
    }
    const footers = document.getElementsByClassName('wizard-footer')
    for (const footer of footers) {
      footer.classList.remove('u-pb-half')
      footer.classList.add('u-pb-2')
    }
  }
})(window, document)
