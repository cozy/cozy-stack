module Helpers
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
  end
end
