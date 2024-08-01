
FWDIR := `pwd`

all: builds

builds:
	go build -o ./dexExecutor ./main.go

clean:
	rm logs/*

run:
	nohup $(FWDIR)/dexExecutor 2>&1 & sleep 1; ps aux | grep "dexExecutor"
