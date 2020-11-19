let fs = require('fs')
let http = require('http')

let fields = JSON.parse(process.env['COZY_FIELDS'])
let credentials = process.env['COZY_CREDENTIALS']
let instance = process.env['COZY_URL']

let payload
try {
  payload = JSON.parse(process.env['COZY_PAYLOAD'])
} catch (e) {
  payload = { error: e }
}

let url = instance + 'data/io.cozy.accounts/' + fields.account
let options = {
  headers: {
    Authorization: `Bearer ${credentials}`,
  },
}

http.get(url, options, (res) => {
  if (res.statusCode !== 200) {
    throw new Error(`Status Code: ${res.statusCode}`)
  }
  res.setEncoding('utf8')
  let rawData = ''
  res.on('data', (chunk) => {
    rawData += chunk
  })
  res.on('end', () => {
    let account = JSON.parse(rawData)
    let data = { account, fields, payload }
    fs.writeFileSync(account.log, JSON.stringify(data, null, '  '))
  })
})
