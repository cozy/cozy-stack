#!/bin/bash

if (( $# == 0 )); then
  echo "Wait some files as argument"
  exit 1
fi


function write_into_until_end_function  () {
  outputfile=$1

  # Retrieve the nth line inside the file
  line=$(sed "${COUNTER}q;d" "$arg")

  while [ $COUNTER -le $fileSize ]
  do
    if [[ "$line" == "}" ]];
    then
      # End of the function

          # do not print the ending brack

          # select the next line
          COUNTER=$((COUNTER+1))
          line=$(sed "${COUNTER}q;d" "$arg")

          # leave the loop
          return
        else
          # Inside the body function

          # print the line
          echo "$line" >> $outputfile

          # select the next line
          COUNTER=$((COUNTER+1))
          line=$(sed "${COUNTER}q;d" "$arg")
    fi
  done
}


mkdir -p /tmp/migration

# Iterate on all file path given in argument.
for arg in "$@"
do
  rm -f "/tmp/migration/test.txt"
  rm -f "/tmp/migration/utils.txt"
  rm -f "/tmp/migration/other.txt"
  rm -f "/tmp/migration/main.txt"
  rm -f "/tmp/migration/out.go"

  # Need to create this file manually because only one of the file inside
  # the package have a `TestMain` function.
  #
  # If a test file doesn't have a TestMain we will need to copy/pasta it
  # manually in a next commit.
  touch /tmp/migration/main.txt
  touch /tmp/migration/utils.txt
  touch /tmp/migration/other.txt
  touch /tmp/migration/test.txt

  COUNTER=1

  printf "===> Starting to migrate %s <===\n" $arg

  # We need a empty line at the end of the file for some match
  echo "" >> $arg

  # Get the number of lines inside the file
  fileSize=`wc -l $arg | cut -d' ' -f1`

  echo "filesize: $fileSize"


  fileTestName=$(echo $(basename $arg) | cut -d'_' -f1 | sed 's/^\(.\)/\U\1/')


  # Loop from 0 to fileSize
  while [ $COUNTER -le $fileSize ]
  do

    # Retrieve the nth line inside the file
    line=$(sed "${COUNTER}q;d" "$arg")

    case $line in
      func\ TestMain*)
        # Skip the proto
        COUNTER=$((COUNTER+1))
        write_into_until_end_function "/tmp/migration/main.txt"
        echo "" >> "/tmp/migration/main.txt"
        ;;

      func\ Test*)
        newProto=$(sed "s/func Test\([a-zA-Z0-9]*\)(t \*testing.T) {/t.Run(\"\1\", func(t *testing.T) {/g" <<< $line)
        testName=$(sed "s/func Test\([a-zA-Z0-9]*\)(t \*testing.T) {/\1/g" <<< $line)
        # echo "Migrate $fileTestName/$testName"
        echo "$newProto" >> "/tmp/migration/test.txt"
        COUNTER=$((COUNTER+1))
        write_into_until_end_function "/tmp/migration/test.txt"
        echo "})" >> "/tmp/migration/test.txt"
        echo "" >> "/tmp/migration/test.txt"
        ;;

      func*)
        write_into_until_end_function  "/tmp/migration/utils.txt"
        echo "}" >> "/tmp/migration/utils.txt"
        echo "" >> "/tmp/migration/utils.txt"
        ;;

      *)
        echo $line >> "/tmp/migration/other.txt"
        COUNTER=$((COUNTER+1))
        ;;
    esac
  done

  cat "/tmp/migration/other.txt" >> "/tmp/migration/out.go"

  printf '%s\n' \
    "func Test${fileTestName}(t *testing.T) {" \
    " if testing.Short() {" \
    " t.Skip(\"an instance is required for this test: test skipped due to the use of --short flag\")" \
    "}" \
    "" >> "/tmp/migration/out.go"

  cat "/tmp/migration/main.txt" >> "/tmp/migration/out.go"
  echo "" >> "/tmp/migration/out.go"

# add the sub tests
cat "/tmp/migration/test.txt" >> "/tmp/migration/out.go"

# close the test
echo "}" >> "/tmp/migration/out.go"
echo "" >> "/tmp/migration/out.go"

# add the helpers at the end
cat "/tmp/migration/utils.txt" >> "/tmp/migration/out.go"

# Replace original file by the newly created file
mv /tmp/migration/out.go $arg

gofmt -s -w $arg

rm -f "/tmp/migration/test.txt"
rm -f "/tmp/migration/utils.txt"
rm -f "/tmp/migration/other.txt"
rm -f "/tmp/migration/main.txt"
rm -f "/tmp/migration/out.go"
done

rm -rf "/tmp/migration"
