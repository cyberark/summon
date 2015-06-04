.PHONY: cucumber

# currently built ad-hoc based on modified buncker
TEST_HARNESS = conjurinc/cauldron-acceptance:v1

cucumber:
	docker run -it --rm -v $(PWD):/cukes -v $(PWD)/../cauldron:/cukes/bin/cauldron -w /cukes $(TEST_HARNESS) cucumber
