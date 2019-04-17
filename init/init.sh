#!/bin/bash
# This script will initialize a Cozy Stack with document containing one row from the iris database
# parameters accepted : number of Cozy / "reini"

# get the cell i,j from a table in csv with sep=","
# parameters file, row, col
getCel(){
	line=$(sed "${2}q;d" $1)
	row=( ${line//,/ } )
	echo ${row[$3]}
}

# generate doc to save in io-cozy-ml
# parameters row
makeDoc(){
	SL=$(getCel iris.csv ${1} 0)
	SW=$(getCel iris.csv ${1} 1)
	PL=$(getCel iris.csv ${1} 2)
	PW=$(getCel iris.csv ${1} 3)
	S=$(getCel iris.csv ${1} 4)

	line="{\"sepal_length\":\"$SL\",\"sepal_width\":\"$SW\",\"petal_length\":\"$PL\",\"petal_width\":\"$PW\",\"species\":\"$S\"}"
	echo -e $line
}

NB_COZY=$1

for ((i=0 ; $NB_COZY - $i ; i++)) do
	# domain name, row in database, creation of json doc
	HOST="user"$i".test.cozy.tools"
	IRIS_ID=$(echo $((i+2)))
	DOC=$(makeDoc $IRIS_ID)
	echo "----------$HOST---------"

	if [ "$2" = "reini" ];
	then
		# Instance reinitialisation
		cozy-stack instances destroy $HOST":8080" --force
		cozy-stack instances add --passphrase cozy --apps home,drive,settings,store $HOST":8080"
	else
		# if the instance exists, we delete ml db
		# else we create instance
		DB_PREFIX=$(cozy-stack instances show-db-prefix $HOST":8080")
		if [ "$DB_PREFIX" = "" ];
		then
			# Cozy's creation and collection of the db prefix
			echo "Instance's creation.."
			cozy-stack instances add --passphrase cozy --apps home,drive,settings,store $HOST":8080"
			DB_PREFIX=$(cozy-stack instances show-db-prefix $HOST":8080")
		else
			# Collection of the db prefix
			curl -X DELETE "http://127.0.0.1:5984/$DB_PREFIX%2Fio-cozy-ml"
		fi
	fi

	# creation of the database io-cozy-ml (%2F stands for /), and creation of the doc
	curl -X PUT "http://127.0.0.1:5984/$DB_PREFIX%2Fio-cozy-ml"
	curl -X POST -H 'Content-Type: application/json' http://127.0.0.1:5984/$DB_PREFIX%2Fio-cozy-ml -d $DOC
	done
