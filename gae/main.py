#!/usr/bin/env python
# coding=utf-8

import calendar
import datetime
import errno
import getpass
import hashlib
import logging
import mimetypes
import optparse
import os
import random
import re
import sys
import tempfile
import time
import urllib
import urllib2
import gzip
import socket
import wsgiref.util

if __name__ == '__main__':
	temp=os.path.dirname(os.path.realpath(__file__))
	deployer_script_path=os.path.join(temp, 'deployer.py')
	execfile(deployer_script_path, globals())