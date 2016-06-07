#!/bin/bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ZF="$DIR/3bf305731dd26307.nzf"

case $1 in
    addzone)
        echo "zone $2 $3" >> $ZF
        touch $DIR/$2.db
        ;;

    delzone)
        rm $DIR/$2.db
        grep -v $2 $ZF > $ZF.new
        mv -f $ZF.new $ZF
        ;;
esac
