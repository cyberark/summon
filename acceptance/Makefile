.PHONY: cucumber

# currently built ad-hoc based on modified buncker
TEST_HARNESS = conjurinc/summon-acceptance:v1

cucumber:
	docker run --rm -v $(PWD):/cukes -v $(PWD)/../summon:/cukes/bin/summon -w /cukes $(TEST_HARNESS) cucumber
