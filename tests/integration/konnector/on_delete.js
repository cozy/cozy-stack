let fs = require('fs')
let http = require('http')

let fields = JSON.parse(process.env['COZY_FIELDS'])
let credentials = process.env['COZY_CREDENTIALS']
let instance = process.env['COZY_URL']

let url =
  instance +
  'data/io.cozy.accounts/' +
  fields.account +
  '?rev=' +
  fields.account_rev
let options = {
  headers: {
    Authorization: `Bearer ${credentials}`
  }
}

const main = () => {
  http.get(url, options, res => {
    if (res.statusCode !== 200) {
      throw new Error(`Status Code: ${res.statusCode}`)
    }
    res.setEncoding('utf8')
    let rawData = ''
    res.on('data', chunk => {
      rawData += chunk
    })
    res.on('end', () => {
      let data = JSON.parse(rawData)
      if (data.failure) {
        throw new Error(data.failure)
      }
      url = instance + 'data/io.cozy.accounts/' + data.relationships.data._id
      http.get(url, options, res2 => {
        if (res2.statusCode !== 200) {
          throw new Error(`Status Code: ${res2.statusCode}`)
        }
        res2.pipe(fs.createWriteStream(data.log))
      })
    })
  })
}

setTimeout(main, 1000)
