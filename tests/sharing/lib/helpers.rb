module Helpers
  SHARED_WITH_ME = "Partagés avec moi".freeze

  @current_dir = File.expand_path "../../tmp", __FILE__

  class <<self
    attr_reader :current_dir, :couch

    def setup
      @couch = Couch.new
    end

    def scenario(scenario = nil)
      if scenario
        @scenario = scenario
        @current_dir = File.expand_path "../../tmp/#{@scenario}", __FILE__
        FileUtils.mkdir_p @current_dir
        RestClient.log = Logger.new "#{@current_dir}/client.log"
      end
      @scenario
    end

    def start_mailhog
      `MailHog --version`
      spawn "MailHog"
    rescue Errno::ENOENT
      puts "MailHog is not installed (or not in the PATH)".yellow
    end

    def spawn(cmd, opts = {})
      log_file_name = opts[:log] || "#{cmd.downcase}.log"
      puts "spawn #{cmd} &> #{log_file_name}".green
      log = "#{@current_dir}/#{log_file_name}"
      pid = Process.spawn cmd, [:out, :err] => [log, File::WRONLY | File::CREAT, 0o644]
      at_exit { Process.kill :SIGINT, pid }
      pid
    end

    def cleanup
      clean_tmp
      couch.clean_test
    end

    def clean_tmp
      tmp = File.expand_path "../../tmp", __FILE__
      FileUtils.cd tmp do
        Dir["*"].each do |f|
          FileUtils.rm_r f
        end
      end
    end

    def fsdiff(a, b)
      diff = `LANG=C diff -qr '#{a}' '#{b}'`
      raise "Diff error" unless $?.success?
      diff.lines.map(&:chomp)
    end

    def db_name(domain, type)
      domain = domain.gsub '.', '-'
      domain = domain.gsub ':', '-'
      type = type.gsub '.', '-'
      db = "#{domain}%2F#{type}"
      db
    end

  end
end
