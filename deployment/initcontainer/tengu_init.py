import os
import signal


def main():
    for key in os.environ.keys():
        print("{} = {}".format(key, os.environ[key]))

    required_vars = os.environ['TENGU_REQUIRED_VARS']

    for var in required_vars.split(','):
        if var not in os.environ:
            print("Var not found ({}) -> BLOCKING".format(var))
            signal.pause()
        
if __name__ == '__main__':
    main()