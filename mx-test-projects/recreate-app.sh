APP_NAME=$1
echo "Appname: ${APP_NAME}"
DEST_DIR=/Users/andrej.koelewijn/GitHub/ModelSDKGo/mx-test-projects
rm -rf $DEST_DIR/${APP_NAME}-app
mkdir $DEST_DIR/${APP_NAME}-app
cd $DEST_DIR/${APP_NAME}-app
"/Applications/Studio Pro 11.6.3 Beta.app/Contents/modeler/mx" create-project --app-name ${APP_NAME}
