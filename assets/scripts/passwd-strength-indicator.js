(function (window, document) {
  const indicator = document.getElementById('password-strength')

  document.getElementById('password').addEventListener('input', function(event) {
    const strength = window.password.getStrength(event.target.value)
    indicator.value = parseInt(strength.percentage, 10)
    indicator.setAttribute('class', 'pw-' + strength.label)
  }, false)
})(window, document)
