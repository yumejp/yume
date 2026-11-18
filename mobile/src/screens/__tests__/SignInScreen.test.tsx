import { fireEvent, render, waitFor } from '@testing-library/react-native';
import { ApiError } from '../../api/client';
import { useServer } from '../../server/ServerContext';
import SignInScreen from '../SignInScreen';

// The first render transforms the whole screen tree; cold runs exceed 5 s.
jest.setTimeout(30_000);

jest.mock('../../server/ServerContext', () => ({ useServer: jest.fn() }));
jest.mock('../../components/ServerConnectionModal', () => ({ ServerConnectionModal: () => null }));

function mockServer(overrides: Record<string, unknown>) {
  const server = {
    url: 'http://100.64.0.1:7654',
    signupOpen: false,
    authNotice: null,
    login: jest.fn().mockResolvedValue(undefined),
    signup: jest.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  (useServer as jest.Mock).mockReturnValue(server);
  return server;
}

it('signs in, and names the cause when the password is wrong', async () => {
  const server = mockServer({
    login: jest.fn().mockRejectedValue(new ApiError('invalid email or password', 401)),
  });
  const view = render(<SignInScreen />);

  // Signup closed: no create-account path, just who can let them in.
  expect(view.queryByText('New here? Create an account')).toBeNull();
  expect(view.getByText(/Ask the person who runs it to let you join/)).toBeTruthy();

  fireEvent.changeText(view.getByLabelText('Email'), ' aki@example.com ');
  fireEvent.changeText(view.getByLabelText('Password'), 'wrong-pass');
  fireEvent.press(view.getByText('Sign in'));

  await waitFor(() => expect(view.getByText(/don’t match an account on this server/)).toBeTruthy());
  expect(server.login).toHaveBeenCalledWith('aki@example.com', 'wrong-pass');
});

it('creates an account only while signup is open, checking the password first', async () => {
  const server = mockServer({ signupOpen: true, authNotice: 'Your session ended. Sign in again to continue.' });
  const view = render(<SignInScreen />);
  expect(view.getByText('Your session ended. Sign in again to continue.')).toBeTruthy();

  fireEvent.press(view.getByText('New here? Create an account'));
  expect(view.getByText('Create your account')).toBeTruthy();

  fireEvent.changeText(view.getByLabelText('Email'), 'aki@example.com');
  fireEvent.changeText(view.getByLabelText('Password'), 'short');
  fireEvent.press(view.getByText('Create account'));
  expect(view.getByText('Use a password of at least 8 characters.')).toBeTruthy();
  expect(server.signup).not.toHaveBeenCalled();

  fireEvent.changeText(view.getByLabelText('Password'), 'long enough');
  fireEvent.press(view.getByText('Create account'));
  await waitFor(() => expect(server.signup).toHaveBeenCalledWith('aki@example.com', 'long enough'));
});
