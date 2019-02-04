(function (window, document) {
  const passInput = document.getElementById('password')
  const indicator = document.getElementById('password-strength')
  const onboardingForm = document.getElementById('onboarding-password-form')

  passInput.addEventListener('input', function(event) {
    const strength = window.password.getStrength(passInput.value)
    indicator.value = parseInt(strength.percentage, 10)
    indicator.setAttribute('class', 'pw-indicator pw-' + strength.label)
    passInput.classList.remove('is-error')
  }, false)

  onboardingForm && onboardingForm.addEventListener('submit', function(event) {
    const label = window.password.getStrength(passInput.value).label
    if (label === 'weak') {
      passInput.classList.add('is-error')
      event.preventDefault()
      return false
    }
  })
})(window, document)
