module Helpers
  SHARED_WITH_ME = "Partages reçus".freeze

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
      # Ignored: on our CI environment, MailHog is started as a docker service
    end

    def cat(filename)
      puts File.read "#{@current_dir}/#{filename}"
    end

    def spawn(cmd, opts = {})
      log_file_name = opts[:log] || "#{cmd.downcase}.log"
      puts "spawn #{cmd} &> #{log_file_name}".green
      log = "#{@current_dir}/#{log_file_name}"
      pid = Process.spawn cmd, [:out, :err] => [log, File::WRONLY | File::CREAT, 0o644]
      if defined? Minitest
        Minitest.after_run { Process.kill :SIGINT, pid }
      else
        at_exit { Process.kill :SIGINT, pid }
      end
      pid
    end

    def cleanup
      clean_tmp
      couch.clean_test
      Email.clear_inbox
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
      if ENV['COZY_SWIFTTEST']
        puts "fsdiff is not available with COZY_SWIFTTEST".yellow
        return []
      end
      diff = `LANG=C diff -qr '#{a}' '#{b}'`
      diff.lines.map(&:chomp)
    end

    def file_exists_in_fs(path)
      File.exist?(path)
    end
  end
end
