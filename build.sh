#!/bin/sh
VERSION="0.17.0"

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
   
   OS="`go env GOOS`"
   ARCH="`go env GOARCH`"
   
   exename=$1
   if [ "$OS" = "windows" ]; then
      exename="$1".exe
   fi
   cp $GSNOVA_DIR/README $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/*.txt $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/bin/main "$GSNOVA_DIR/$DIST_DIR/$exename"
   cp $GSNOVA_DIR/conf/"$1".conf $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/conf/hosts.conf $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/conf/Fake* $GSNOVA_DIR/$DIST_DIR/cert
   cp $GSNOVA_DIR/conf/spac.json $GSNOVA_DIR/$DIST_DIR/spac
   cp $GSNOVA_DIR/conf/user-gfwlist.txt $GSNOVA_DIR/$DIST_DIR/spac
   cp -r $GSNOVA_DIR/web $GSNOVA_DIR/$DIST_DIR
   zip -r "$1"_"$VERSION"_"$OS"_"$ARCH".zip $DIST_DIR/*
   rm -rf $GSNOVA_DIR/$DIST_DIR
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
