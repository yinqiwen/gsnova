#!/bin/sh
VERSION="0.21.0"

GSNOVA_DIR=`dirname $0 | sed -e "s#^\\([^/]\\)#${PWD}/\\1#"` # sed makes absolute
build_product()
{
   export GOPATH="$GSNOVA_DIR"
   go get -u github.com/yinqiwen/godns
   cd src
   rm common/constants.go
   echo "package common" >> common/constants.go
   echo "var Version string = \"$VERSION\"" >> common/constants.go
   echo "var Product string = \"$1\"" >> common/constants.go
   
   go install -v ...
}

build_dist()
{
   build_product $*
   if test $? -eq 1; then
	  echo "Build gsnova failed!"
	  return 1
   fi  
   
   cd $GSNOVA_DIR
   DIST_DIR="$1"-"$VERSION"
   mkdir -p $GSNOVA_DIR/$DIST_DIR/cert
   mkdir -p $GSNOVA_DIR/$DIST_DIR/spac
   mkdir -p $GSNOVA_DIR/$DIST_DIR/hosts
   
   OS="`go env GOOS`"
   ARCH="`go env GOARCH`"
   
   exename="$1"
   if [ "$OS" = "windows" ]; then
      exename="$1".exe
   fi
   cp $GSNOVA_DIR/README.md $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/*.txt $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/bin/"$OS"_$ARCH/main* $GSNOVA_DIR/$DIST_DIR/$exename
   cp $GSNOVA_DIR/conf/"$1".conf $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/conf/*_hosts.conf $GSNOVA_DIR/$DIST_DIR/hosts
   cp $GSNOVA_DIR/conf/Fake* $GSNOVA_DIR/$DIST_DIR/cert
   cp $GSNOVA_DIR/conf/*_spac.json $GSNOVA_DIR/$DIST_DIR/spac
   cp $GSNOVA_DIR/conf/user-gfwlist.txt $GSNOVA_DIR/$DIST_DIR/spac
   cp -r $GSNOVA_DIR/web $GSNOVA_DIR/$DIST_DIR
   if [ "$OS" = "windows" ]; then
      zip -r "$1"_"$VERSION"_"$OS"_"$ARCH".zip ${1}-${VERSION}/*
   else
      chmod 744 $DIST_DIR/gsnova
      chmod 600 $DIST_DIR/gsnova.conf
      chmod 644 $DIST_DIR/*.txt
      chmod -R 744 $DIST_DIR/cert/*
      chmod -R 744 $DIST_DIR/hosts/*
      chmod -R 744 $DIST_DIR/spac/*
      chmod -R 744 $DIST_DIR/web/*
      tar czf ${1}_${VERSION}_${OS}_${ARCH}.tar.gz ${1}-${VERSION}
   fi
   rm -rf $GSNOVA_DIR/$DIST_DIR
}

main()
{
    if [ "x$1" = "xdist" ]; then
	    shift
            export CGO_ENABLED=0
            export GOOS=linux
            export GOARCH=amd64
            build_dist gsnova
            export GOARCH=arm
            build_dist gsnova
            export GOOS=windows
            export GOARCH=amd64
            build_dist gsnova
            export GOARCH=386
            build_dist gsnova
	else
		build_product gsnova
	fi	
}

main $*
