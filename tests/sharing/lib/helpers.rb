module Helpers
  @current_dir = File.expand_path "../../tmp", __FILE__

  class <<self
    attr_reader :current_dir
  end

  def self.scenario(scenario = nil)
    if scenario
      @scenario = scenario
      @current_dir = File.expand_path "../../tmp/#{@scenario}", __FILE__
      FileUtils.mkdir_p @current_dir
    end
    @scenario
  end

  def self.start_mailhog
    `MailHog --version`
    spawn "MailHog"
  rescue Errno::ENOENT
    puts "MailHog is not installed (or not in the PATH)".yellow
  end

  def self.spawn(cmd, opts = {})
    log_file_name = opts[:log] || "#{cmd.downcase}.log"
    puts "spawn #{cmd} &> #{log_file_name}".green
    log = "#{@current_dir}/#{log_file_name}"
    pid = Process.spawn cmd, [:out, :err] => [log, File::WRONLY | File::CREAT, 0o644]
    at_exit { Process.kill :SIGINT, pid }
    pid
  end
end
