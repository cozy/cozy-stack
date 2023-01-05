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


TMPDIR=`mktemp -d`

# Iterate on all file path given in argument.
for arg in "$@"
do
  rm -f "$TMPDIR/test.txt"
  rm -f "$TMPDIR/utils.txt"
  rm -f "$TMPDIR/other.txt"
  rm -f "$TMPDIR/main.txt"
  rm -f "$TMPDIR/out.go"

  # Need to create this file manually because only one of the file inside
  # the package have a `TestMain` function.
  #
  # If a test file doesn't have a TestMain we will need to copy/pasta it
  # manually in a next commit.
  touch "$TMPDIR/main.txt"
  touch "$TMPDIR/utils.txt"
  touch "$TMPDIR/other.txt"
  touch "$TMPDIR/test.txt"

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
        write_into_until_end_function "$TMPDIR/main.txt"
        echo "" >> "$TMPDIR/main.txt"
        ;;

      func\ Test*)
        newProto=$(sed "s/func Test\([a-zA-Z0-9]*\)(t \*testing.T) {/t.Run(\"\1\", func(t *testing.T) {/g" <<< $line)
        testName=$(sed "s/func Test\([a-zA-Z0-9]*\)(t \*testing.T) {/\1/g" <<< $line)
        # echo "Migrate $fileTestName/$testName"
        echo "$newProto" >> "$TMPDIR/test.txt"
        COUNTER=$((COUNTER+1))
        write_into_until_end_function "$TMPDIR/test.txt"
        echo "})" >> "$TMPDIR/test.txt"
        echo "" >> "$TMPDIR/test.txt"
        ;;

      func*)
        write_into_until_end_function  "$TMPDIR/utils.txt"
        echo "}" >> "$TMPDIR/utils.txt"
        echo "" >> "$TMPDIR/utils.txt"
        ;;

      *)
        echo $line >> "$TMPDIR/other.txt"
        COUNTER=$((COUNTER+1))
        ;;
    esac
  done

  cat "$TMPDIR/other.txt" >> "$TMPDIR/out.go"

  printf '%s\n' \
    "func Test${fileTestName}(t *testing.T) {" \
    " if testing.Short() {" \
    " t.Skip(\"an instance is required for this test: test skipped due to the use of --short flag\")" \
    "}" \
    "" >> "$TMPDIR/out.go"

  cat "$TMPDIR/main.txt" >> "$TMPDIR/out.go"
  echo "" >> "$TMPDIR/out.go"

# add the sub tests
cat "$TMPDIR/test.txt" >> "$TMPDIR/out.go"

# close the test
echo "}" >> "$TMPDIR/out.go"
echo "" >> "$TMPDIR/out.go"

# add the helpers at the end
cat "$TMPDIR/utils.txt" >> "$TMPDIR/out.go"

# Replace original file by the newly created file
mv $TMPDIR/out.go $arg

gofmt -s -w $arg

rm -f "$TMPDIR/test.txt"
rm -f "$TMPDIR/utils.txt"
rm -f "$TMPDIR/other.txt"
rm -f "$TMPDIR/main.txt"
rm -f "$TMPDIR/out.go"
done

rm -rf "$TMPDIR"
