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

def get_email():
	print '==================Snova(Go)Deployer v0.17.2==================='
	n = raw_input('Specify google email account first? (y/n, default n):')
	if n == 'y' or n == 'Y':
		email = raw_input("Email: ").strip()
		if len(email) > 0:
			return ["-e", email, "--no_cookies"]
	return []

def get_action():
	action = raw_input('Enter your action?(0:update/1:rollback, default 0):').strip()
	if len(action) == 0 or action=='0':
		return 'update'
	if action=='1':
		return 'rollback'
	print "[WARN]:Invalid action choice:%s, use default 'update' instead" % (action)
	return 'update'


if __name__ == '__main__':
	temp=os.path.dirname(os.path.realpath(__file__))
	appcfg_script_path=os.path.join(temp, 'appcfg', 'appcfg.py')
	sys.path = [os.path.join(temp, 'appcfg')] + sys.path
	if len(sys.argv) == 1:
		email = get_email()
		action = get_action()
		deploy_args = ["--skip_sdk_update_check", action, os.path.join(temp, 'src')] + email
		print "Enter appid, use ',' as separator if you have more than 1 appid."
		appids = raw_input('AppID: ')
		for appid in appids.split(','):
			appid = appid.strip()
			app_args = ['-A', appid]
			tmp = appid.split('.')
			#version in appid
			if len(tmp) == 2:
				app_args = ['-A', tmp[0], '-V', tmp[1]]
			sys.argv = deploy_args + app_args
			print '==============Start %s AppID:%s===============' % (action, appid)
			execfile(appcfg_script_path, globals())
			print '==============End %s AppID:%s==============='% (action, appid)
		raw_input("Enter to exit:")
		sys.exit(0)
		#sys.argv = sys.argv + deploy_args
	execfile(appcfg_script_path, globals())