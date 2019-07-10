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

  let localStorage = null
  try {
    localStorage = window.localStorage
  } catch (e) {
    // do nothing
  }

  // Set the trusted device token from the localstorage in the form if it exists
  const twoFactorTrustedDeviceToken =
    (localStorage && localStorage.getItem(twoFactorTrustedDeviceTokenKey)) || ''
  twoFactorTrustedDomainInput.value = twoFactorTrustedDeviceToken

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
