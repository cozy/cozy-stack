(function (d) {
  var passwordVisibility = false
  var passwordInput = d.getElementById('password')
  var passwordVisibilityButton = d.getElementById('password-visibility-button')
  passwordVisibilityButton.addEventListener('click', function (event) {
    event.preventDefault()
    passwordVisibility = !passwordVisibility
    passwordInput.type = passwordVisibility ? 'text' : 'password'
    passwordInput.setAttribute('autocomplete', passwordVisibility ? 'off' : 'current-password')

    passwordVisibilityButton.textContent = passwordVisibility
      ? passwordVisibilityButton.getAttribute('data-hide')
      : passwordVisibilityButton.getAttribute('data-show')
  })
})(window.document)
