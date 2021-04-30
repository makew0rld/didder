GITV  != git describe --tags
GITC  != git rev-parse --verify HEAD
SRC   != find . -type f -name '*.go' ! -name '*_test.go'
TEST  != find . -type f -name '*_test.go'
DATEC != date +'%B %d, %Y'

PREFIX  ?= /usr/local
VERSION ?= $(GITV)
COMMIT  ?= $(GITC)
BUILDER ?= Makefile
DATE    ?= $(DATEC)

GO      := go
INSTALL := install
RM      := rm
SED     := sed
PANDOC  := pandoc
GZIP    := gzip
MANDB   := mandb

didder: go.mod go.sum $(SRC)
	GO111MODULE=on CGO_ENABLED=0 $(GO) build -o $@ -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.builtBy=$(BUILDER)"

.PHONY: clean
clean:
	$(RM) -f didder

.PHONY: man
man:
	$(PANDOC) didder.1.md -s -t man -o didder.1
	$(SED) -i 's/VERSION/$(VERSION)/g' didder.1
	$(SED) -i 's/DATE/$(DATE)/g' didder.1
	$(PANDOC) -f man didder.1 -o MANPAGE.md
	$(SED) -i '1s/^/<!-- DO NOT EDIT, AUTOMATICALLY GENERATED, EDIT dither.1.md INSTEAD -->\n/' MANPAGE.md
	$(SED) -i 's/:   //g' MANPAGE.md
	$(SED) -i 's/    //g' MANPAGE.md

.PHONY: install
install: didder
	$(INSTALL) -d $(PREFIX)/bin/
	$(INSTALL) -m 755 didder $(PREFIX)/bin/didder
	$(GZIP) -c didder.1 > /usr/share/man/man1/didder.1.gz
	mandb

.PHONY: uninstall
uninstall:
	$(RM) -f $(PREFIX)/bin/didder

# Development helpers

.PHONY: fmt
fmt:
	$(GO) fmt ./...
