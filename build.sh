#!/bin/sh
VERSION="0.15.0beta"

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
   if $cygwin; then
      exename=gsnova.exe
   fi
   cp $GSNOVA_DIR/README $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/*.txt $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/bin/main* $GSNOVA_DIR/$DIST_DIR/$exename
   cp $GSNOVA_DIR/conf/*.conf $GSNOVA_DIR/$DIST_DIR
   cp $GSNOVA_DIR/conf/Fake* $GSNOVA_DIR/$DIST_DIR/cert
   zip -r GSnova_"$VERSION"_"$OS"_"$ARCH".zip $DIST_DIR/*
   rm -rf $GSNOVA_DIR/$DIST_DIR
}

main()
{
    if [ "x$1" == "xdist" ]; then
	    shift
		build_dist $*
	else
		build_gsnova $*
	fi	
}

main $*