;(function (w) {
  var url = w.location.host + '/move/importing/realtime'
  if (w.location.protocol === 'http:') {
    url = 'ws://' + url
  } else {
    url = 'wss://' + url
  }
  var socket = new WebSocket(url)
  socket.addEventListener('message', function (event) {
    var data = JSON.parse(event.data)
    w.location = data.redirect
  })
})(window)
