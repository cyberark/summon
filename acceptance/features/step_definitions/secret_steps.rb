Given(/^a secret "([^"]*)" with "([^"]*)"$/) do |name, value|
	steps %{
		Given I append to "provider" with:
			"""
			if [ "$1" == "#{name}" ]; then
				echo "#{value}"
				exit 0
			fi

			"""
	}
end

Given(/^a secret "([^"]*)" with:$/) do |name, string|
	steps %{
		Given I append to "provider" with:
			"""
			if [ "$1" == "#{name}" ]; then
				cat << __EOF____
			#{string}
			__EOF____
				exit 0
			fi

			"""
	}
end

Given(/^other secrets don't exist$/) do
	steps %{
		Given I append to "provider" with:
			"""
			echo "Secret $1 doesn't exist!"
			exit 1
			"""
	}
end

Given(/^a non-existent secret "([^"]*)"$/) do |name|
	steps %{
		Given I append to "provider" with:
			"""
			if [ "$1" == "#{name}" ]; then
        	    echo "Error fetching variable #{name}" >&2
        	    exit 1
			fi
			"""
	}
end