function listenToRealtimeOAuthClientsUpdates(w, d) {
  const token = d.querySelector('body').dataset.token

  let url = w.location.host + '/realtime/'
  if (w.location.protocol === 'http:') {
    url = 'ws://' + url
  } else {
    url = 'wss://' + url
  }

  const socket = new WebSocket(url)
  socket.onopen = () => {
    const authMsg = JSON.stringify({ method: 'AUTH', payload: token })
    socket.send(authMsg)
    const subscribeMsg = JSON.stringify({ method: 'SUBSCRIBE', payload: { type: 'io.cozy.oauth.clients' } })
    socket.send(subscribeMsg)
  }
  socket.onerror = (err) => {
    console.error(err)
  }
  socket.onmessage = (message) => {
    const { event } = JSON.parse(message.data)

    if (event === 'DELETED') {
      w.location.reload()
    }
  }
}

(function (w, d) {
  listenToRealtimeOAuthClientsUpdates(w, d)

  const refreshBtn = d.querySelector('#refresh-btn')
  refreshBtn.addEventListener('click', () => {
    window.location.reload()
  })
})(window, document)
