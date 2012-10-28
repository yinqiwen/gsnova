# -*- mode: python -*-
a = Analysis(['deployer.py'],
             pathex=['C:\\Src\\GoProjects\\gsnova\\gae'],
             hiddenimports=[],
             hookspath=None)
pyz = PYZ(a.pure)
exe = EXE(pyz,
          a.scripts,
          a.binaries,
          a.zipfiles,
          a.datas,
          name=os.path.join('dist', 'deployer.exe'),
          debug=False,
          strip=None,
          upx=True,
          console=True )
