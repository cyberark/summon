When(/^I wait (\d+) seconds?$/) do |seconds|
	sleep(seconds.to_i)
end

# aruba's built-in file-content matchers ("the file ... should contain" /
# "should not exist") call stop_all_commands as a side effect (they assume a
# command writes its output and then exits) — fatal here, since it kills the
# still-running daemon mid-scenario. These check the filesystem directly
# instead, without touching the running command.

Then(/^the file "([^"]*)" should already contain "([^"]*)"$/) do |file_name, expected|
	path = expand_path(file_name)
	content = File.exist?(path) ? File.read(path) : ""
	unless content.include?(expected)
		raise "Expected #{file_name.inspect} to already contain #{expected.inspect}; actual content: #{content.inspect}"
	end
end

Then(/^the file "([^"]*)" should not exist yet$/) do |file_name|
	path = expand_path(file_name)
	if File.exist?(path)
		raise "Expected #{file_name.inspect} not to exist yet; actual content: #{File.read(path).inspect}"
	end
end

# Daemon-mode polls on its own interval, so waiting for a rotation's effect
# needs to tolerate that interval plus provider/exec-hook overhead — a fixed
# sleep is too tight. Poll the file instead of sleeping a fixed duration.
When(/^I wait for the file "([^"]*)" to contain "([^"]*)"$/) do |file_name, expected|
	start_time = Time.now
	loop do
		content = File.exist?(expand_path(file_name)) ? File.read(expand_path(file_name)) : ""
		break if content.include?(expected)

		if Time.now - start_time > aruba.config.exit_timeout
			raise "Operation timed out waiting for file #{file_name.inspect} to contain #{expected.inspect}; actual content: #{content.inspect}"
		end

		sleep 0.2
	end
end

Then(/^the file "([^"]*)" should already have (\d+) lines?$/) do |file_name, count|
	path = expand_path(file_name)
	lines = File.exist?(path) ? File.readlines(path) : []
	unless lines.length == count.to_i
		raise "Expected #{file_name.inspect} to have #{count} line(s); actual content: #{lines.join.inspect}"
	end
end
