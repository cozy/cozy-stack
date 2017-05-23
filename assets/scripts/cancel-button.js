(function (w) {
  var cancel = w.document.querySelector('button[type=cancel]')
  cancel.addEventListener('click', function (event) {
    event.preventDefault()
    if (w.history.length > 1) {
      w.history.back()
    } else {
      w.location.pathname = '/'
    }
  })
})(window)
