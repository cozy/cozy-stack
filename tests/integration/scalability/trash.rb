require_relative '../boot'

stack_port = 8080
domain = 'trash.test.cozy.tools:8080'
email = 'me@cozy.tools'

stack = Stack.get stack_port
inst = Instance.new stack, domain: domain, email: email

stack.create_instance inst

(0...100).map do |i|
  Thread.new do
    folder = Folder.create inst, name: "foo-#{i}"
    100.times do |j|
      CozyFile.create inst, name: "bar-#{j}", dir_id: folder.couch_id
    end
    folder.remove inst
  end
end.map(&:value)

before = Time.now
Folder.clear_trash inst
after = Time.now
ap "Clearing the trash took #{after - before}s"

ap "Don't forget to destroy the instance"
ap "cozy-stack instances rm #{domain}"
