;(function (d) {
  var visible = false
  var input = d.getElementById('password')
  var button = d.getElementById('password-visibility-button')
  var icon = d.getElementById('password-visibility-icon')

  button.addEventListener('click', function (event) {
    event.preventDefault()
    visible = !visible
    input.type = visible ? 'text' : 'password'
    input.setAttribute('autocomplete', visible ? 'off' : 'current-password')

    if (visible) {
      icon.classList.remove('icon-eye-closed')
      icon.classList.add('icon-eye-opened')
    } else {
      icon.classList.remove('icon-eye-opened')
      icon.classList.add('icon-eye-closed')
    }

    button.setAttribute(
      'title',
      visible
        ? button.getAttribute('data-hide')
        : button.getAttribute('data-show')
    )

    input.focus()
  })
})(window.document)
