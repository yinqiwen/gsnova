#!/usr/bin/env bash

VERSION="0.23.0"

#this part is copied from ANT's script
# OS specific support.  $var _must_ be set to either true or false.
cygwin=false;
case "`uname`" in
  CYGWIN*) cygwin=true ;;
esac

GSNOVA_DIR=`dirname $0 | sed -e "s#^\\([^/]\\)#${PWD}/\\1#"` # sed makes absolute
if $cygwin; then
  if [ "$OS" = "Windows_NT" ] && cygpath -m .>/dev/null 2>/dev/null ; then
    format=mixed
  else
    format=windows
  fi
  GSNOVA_DIR=`cygpath --path --$format "$GSNOVA_DIR"`
fi

build_product()
{
   export GOPATH="$GSNOVA_DIR"
   cd src
   #mv common/constants.go{,.bak}
   rm common/constants.go
   echo "package common" >> common/constants.go
   echo "var Version string = \"$VERSION\"" >> common/constants.go
   echo "var Product string = \"$1\"" >> common/constants.go
   go install -v ...
   #mv common/constants.go{.bak,}
}

build_dist()
{
   build_product $*
   if test $? -eq 1; then
	  echo "Build gsnova failed!"
	  return 1
   fi  
   
   cd $GSNOVA_DIR
   DIST_DIR=$GSNOVA_DIR/${1}-${VERSION}
   mkdir -p $DIST_DIR/conf
   mkdir -p $DIST_DIR/hosts
   
   OS="`go env GOOS`"
   ARCH="`go env GOARCH`"
   
   exename=$1
   if [ "$OS" = "windows" ]; then
      exename="$1".exe
      cp $GSNOVA_DIR/misc/hidegsnova.exe $DIST_DIR
   fi
   cp $GSNOVA_DIR/README.md $DIST_DIR
   cp $GSNOVA_DIR/*.txt $DIST_DIR
   cp $GSNOVA_DIR/bin/main $DIST_DIR/$exename
   cp $GSNOVA_DIR/conf/hosts.conf $DIST_DIR/hosts
   cp $GSNOVA_DIR/conf/Fake* $DIST_DIR/conf
   cp $GSNOVA_DIR/conf/spac.json $DIST_DIR/conf
   cp $GSNOVA_DIR/conf/user-gfwlist.txt $DIST_DIR/conf
   cp $GSNOVA_DIR/conf/gsnova.conf $DIST_DIR/conf
   cp -R $GSNOVA_DIR/web $DIST_DIR
   if [ "$OS" = "windows" ]; then
      zip -r "$1"_"$VERSION"_"$OS"_"$ARCH".zip ${1}-${VERSION}/*
   else
      chmod 744 $DIST_DIR/gsnova
      chmod -R 644 $DIST_DIR/conf/*
      #chmod -R 644 $DIST_DIR/web/*
      tar czf ${1}_${VERSION}_${OS}_${ARCH}.tar.gz ${1}-${VERSION}
   fi
   rm -rf $DIST_DIR
}

main()
{
    if [ "x$1" = "xdist" ]; then
	    shift
        build_dist $*
	else
		build_product $*
	fi	
}

main $*
