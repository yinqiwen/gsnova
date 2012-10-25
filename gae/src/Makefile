DIRS := codec\
        se\
		snappy\
		event\
		service\
		handler\
		entry
		
all:
	@for dir in $(DIRS); do make -C $$dir; echo; done

clean:
	@for dir in $(DIRS); do make -C $$dir clean; echo; done

