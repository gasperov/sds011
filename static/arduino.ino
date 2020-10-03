/*
 * RX is digital pin 10 (connect to TX of other device)
 * TX is digital pin 11 (connect to RX of other device)

 */
#include <SoftwareSerial.h>

SoftwareSerial mySerial(10, 11); // RX, TX

void setup() {
  Serial.begin(57600);
  while (!Serial) {
    ; // wait for serial port to connect. Needed for native USB port only
  }

  Serial.println("Goodnight moon!");
  mySerial.begin(9600);
}

struct sds_011_data {
	int len;
	float pm10, pm25;
	int pm10_serial;
	int pm25_serial;
	int checksum_is;
	int checksum_ok;
   
	sds_011_data() {
		len = 0;
		pm10 = pm25 = 0.0;
		todo = false;

		pm10_serial = 0;
		pm25_serial = 0;
		checksum_is;
		checksum_ok = 0;   
	}

	bool decode(int value) {
		switch (len) {
			case (0): if (value != 170) { len = -1; }; break;
			case (1): if (value != 192) { len = -1; }; break;
			case (2): pm25_serial = value; checksum_is = value; break;
			case (3): pm25_serial += (value << 8); checksum_is += value; break;
			case (4): pm10_serial = value; checksum_is += value; break;
			case (5): pm10_serial += (value << 8); checksum_is += value; break;
			case (6): checksum_is += value; break;
			case (7): checksum_is += value; break;
			case (8): if (value == (checksum_is % 256)) { checksum_ok = 1; } else { len = -1; }; break;
			case (9): if (value != 171) { len = -1; }; break;
		}
		len++;
		if (len == 10 && checksum_ok == 1) {
			pm10 = (float)pm10_serial/10.0;
			pm25 = (float)pm25_serial/10.0;
			len = 0; checksum_ok = 0; pm10_serial = 0.0; pm25_serial = 0.0; checksum_is = 0;        
			return true;
		}     

		return false;
	}
} pm;

void loop() {
	if (mySerial.available()) {
		//    Serial.println(mySerial.read());
		if (pm.decode(mySerial.read())) {
			Serial.print("ug/m3 ");
			Serial.print("PM2.5=");
			Serial.print(pm.pm25);
			Serial.print(", PM10=");
			Serial.println(pm.pm10);      
		}
	}
	if (Serial.available()) {
		Serial.read();
		//mySerial.write(Serial.read());
	}
}
