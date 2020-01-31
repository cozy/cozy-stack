;(function(d) {
  if (d.body.clientWidth > 1024) {
    const avatars = d.getElementsByClassName('c-avatar')
    for (const avatar of avatars) {
      avatar.classList.add('c-avatar--xlarge')
    }
  }
})(document)
