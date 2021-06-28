;(function (w, d) {
  const form = d.getElementsByTagName('form')[0]
  const passInput = d.getElementById('password')
  const passTip = d.getElementById('password-tooltip')
  const indicator = d.getElementById('password-strength')

  passInput.addEventListener(
    'input',
    function () {
      const strength = w.password.getStrength(passInput.value)
      const pct = Math.round(parseInt(strength.percentage, 10) / 4) * 4
      indicator.setAttribute('aria-valuenow', pct)
      indicator.setAttribute(
        'class',
        `progress-bar w-${pct} pass-${strength.label}`
      )
      passInput.classList.remove('is-invalid')
      passTip && passTip.classList.remove('text-error')
    },
    false
  )

  form.addEventListener('submit', function (event) {
    const strength = w.password.getStrength(passInput.value)
    if (strength.label === 'weak') {
      passInput.classList.add('is-invalid')
      passTip && passTip.classList.add('text-error')
      event.preventDefault()
      event.stopPropagation()
    }
  })
})(window, document)
