class Contact
  include Model

  attr_reader :name, :fullname, :emails, :addresses, :phones, :cozy, :me

  def self.doctype
    "io.cozy.contacts"
  end

  def initialize(opts = {})
    first = opts[:given_name] || Faker::Name.first_name
    last = opts[:family_name] || Faker::Name.last_name
    @name = opts[:name] || { given_name: first, familyName: last }
    @fullname = opts[:fullname] || "#{first} #{last}"

    email = opts[:email] || Faker::Internet.email([first, last, @fullname].sample)
    @emails = opts[:emails] || [{ address: email }]

    @addresses = opts[:addresses] || [{
      street: opts[:street] || Faker::Address.street_name,
      city: opts[:city] || Faker::Address.city,
      post_code: opts[:post_code] || Faker::Address.postcode
    }]

    phone = opts[:phone] || Faker::PhoneNumber.cell_phone
    @phones = [{ number: phone }]
    @cozy = opts[:cozy]
    @me = opts[:me] || false
  end

  def self.find(inst, id)
    opts = {
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/data/#{doctype}/#{id}"].get opts
    from_json JSON.parse(res.body)
  end

  def self.all(inst)
    opts = {
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/data/#{doctype}/_all_docs?include_docs=true"].get opts
    JSON.parse(res.body)["rows"]
        .reject { |r| r["id"] =~ /^_design/ }
        .map { |r| from_json r["doc"] }
  end

  def self.from_json(j)
    contact = Contact.new(
      name: j["name"] || {},
      fullname: j["fullname"] || "",
      emails: j["email"] || "",
      cozy: j["cozy"] || [],
      addresses: j["address"] || [],
      phone: j["phone"] || "",
      me: j["me"]
    )
    contact.couch_id = j["_id"]
    contact.couch_rev = j["_rev"]
    contact
  end

  def primary_email
    @emails.dig 0, :address
  end

  def as_json
    {
      name: @name,
      fullname: @fullname,
      email: @emails,
      cozy: @cozy,
      address: @addresses,
      phone: @phones
    }
  end
end
