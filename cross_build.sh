#!/bin/sh
VERSION="0.16.0"

GSNOVA_DIR=`dirname $0 | sed -e "s#^\\([^/]\\)#${PWD}/\\1#"` # sed makes absolute
build_gsnova()
{
   export GOPATH="$GSNOVA_DIR"
   cd src
   rm common/version.go
   echo "package common" >> common/version.go
   echo "var Version string = \"$VERSION\"" >> common/version.go
   go install -v ...
}

build_dist()
{
   build_gsnova $*
   if test $? -eq 1; then
	  echo "Build gsnova failed!"
	  return 1
   fi  
   
   cd $GSNOVA_DIR
   DIST_DIR=gsnova-"$VERSION"
   mkdir -p $GSNOVA_DIR/$DIST_DIR/cert
   
   OS="`go env GOOS`"
   ARCH="`go env GOARCH`"
   
   exename=gsnova
   if [ "$OS" = "windows" ]; then
      exename=gsnova.exe
   fi
   cp $GSNOVA_DIR/README $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/*.txt $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/bin/"$OS"_$ARCH/main* $GSNOVA_DIR/$DIST_DIR/$exename
   cp $GSNOVA_DIR/conf/*.conf $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/conf/Fake* $GSNOVA_DIR/$DIST_DIR/cert
   cp $GSNOVA_DIR/conf/spac.json $GSNOVA_DIR/$DIST_DIR
   cp -r $GSNOVA_DIR/web $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/conf/user-gfwlist.txt $GSNOVA_DIR/$DIST_DIR
   zip -r gsnova_"$VERSION"_"$OS"_"$ARCH".zip $DIST_DIR/*
   rm -rf $GSNOVA_DIR/$DIST_DIR
}

main()
{
    if [ "x$1" = "xdist" ]; then
	    shift
            export CGO_ENABLED=0
            export GOOS=windows
            export GOARCH=386
            build_dist $*
            export GOARCH=amd64
            build_dist $*
	else
		build_gsnova $*
	fi	
}

main $*
