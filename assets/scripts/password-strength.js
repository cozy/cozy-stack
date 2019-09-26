;(function(w, d) {
  const form = d.getElementsByTagName('form')[0]
  const passInput = d.getElementById('password')
  const indicator = d.getElementById('password-strength')
  const passTip = d.getElementById('password-tip')
  const submitButton = form.querySelector('button')

  passInput.addEventListener(
    'input',
    function() {
      const strength = w.password.getStrength(passInput.value)
      indicator.value = parseInt(strength.percentage, 10)
      indicator.setAttribute('class', 'pw-indicator pw-' + strength.label)
      passInput.classList.remove('is-error')
      passTip && passTip.classList.remove('u-pomegranate')
      if (strength.label === 'weak') {
        submitButton.setAttribute('disabled', '')
      } else {
        submitButton.removeAttribute('disabled')
      }
    },
    false
  )
  passInput.focus()

  form.addEventListener('submit', function(event) {
    const strength = w.password.getStrength(passInput.value)
    if (strength.label === 'weak') {
      passInput.classList.add('is-error')
      passTip && passTip.classList.add('u-pomegranate')
      event.preventDefault()
      event.stopPropagation()
    }
  })
})(window, document)
